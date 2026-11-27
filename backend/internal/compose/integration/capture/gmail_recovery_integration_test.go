// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Recovering mail a sync never read. A Gmail sync whose history cursor expired
// lists only the newest messages. An owner's follow-up sent before them is then
// missing from the contact, and the meeting it answered still asks for one.
// A history import over the gap reads it back through the same capture path,
// and reads nothing twice.

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const recoveryOwner = "rep@myco.example"

// gmailNote is one message in the stub mailbox.
type gmailNote struct {
	id, from, to, subject string
	at                    time.Time
	sent                  bool
}

// headers is the message's RFC822 header block.
func (n gmailNote) headers() []string {
	return []string{
		"From: " + n.from, "To: " + n.to, "Subject: " + n.subject,
		"Date: " + n.at.Format(time.RFC1123Z), "Message-ID: <" + n.id + "@mail.example>",
		"Content-Type: text/plain; charset=utf-8",
	}
}

func (n gmailNote) labels() []string {
	if n.sent {
		return []string{"SENT"}
	}
	return []string{"INBOX"}
}

// listedGmail is a stubbed Gmail mailbox that answers as Gmail does: newest
// first, at most maxResults ids, an after: query and a page token honored. Its
// history API has forgotten every cursor, so each incremental sync falls back
// to the recent listing.
type listedGmail struct {
	mu    sync.Mutex
	notes []gmailNote
}

func (g *listedGmail) deliver(notes ...gmailNote) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.notes = append(g.notes, notes...)
}

// listing answers one messages.list call.
func (g *listedGmail) listing(r *http.Request) map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	newest := slices.Clone(g.notes)
	slices.SortFunc(newest, func(a, b gmailNote) int { return b.at.Compare(a.at) })
	if after, ok := strings.CutPrefix(r.URL.Query().Get("q"), "after:"); ok {
		since, err := time.Parse("2006/01/02", after)
		if err == nil {
			newest = slices.DeleteFunc(newest, func(n gmailNote) bool { return n.at.Before(since) })
		}
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
	size, err := strconv.Atoi(r.URL.Query().Get("maxResults"))
	if err != nil || size <= 0 {
		size = len(newest)
	}
	end := min(offset+size, len(newest))
	listed := make([]map[string]string, 0, end-offset)
	for _, n := range newest[min(offset, end):end] {
		listed = append(listed, map[string]string{"id": n.id})
	}
	out := map[string]any{"messages": listed}
	if end < len(newest) {
		out["nextPageToken"] = strconv.Itoa(end)
	}
	return out
}

// message answers one messages.get call, in the format it asked for.
func (g *listedGmail) message(r *http.Request) (map[string]any, bool) {
	id := strings.TrimPrefix(r.URL.Path, "/messages/")
	g.mu.Lock()
	defer g.mu.Unlock()
	i := slices.IndexFunc(g.notes, func(n gmailNote) bool { return n.id == id })
	if i < 0 {
		return nil, false
	}
	n := g.notes[i]
	out := map[string]any{"id": id, "labelIds": n.labels(), "internalDate": strconv.FormatInt(n.at.UnixMilli(), 10)}
	if r.URL.Query().Get("format") == "metadata" {
		headers := []map[string]string{}
		for _, line := range n.headers() {
			name, value, _ := strings.Cut(line, ": ")
			headers = append(headers, map[string]string{"name": name, "value": value})
		}
		out["payload"] = map[string]any{"headers": headers}
		return out, true
	}
	rfc822 := strings.Join(n.headers(), "\r\n") + "\r\n\r\nHello."
	out["raw"] = base64.RawURLEncoding.EncodeToString([]byte(rfc822))
	return out, true
}

func (g *listedGmail) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"access_token": "access-tok", "refresh_token": "refresh-tok", "expires_in": 3599})
	})
	mux.HandleFunc("/profile", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"emailAddress": recoveryOwner, "historyId": "1001"})
	})
	mux.HandleFunc("/history", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) { integration.WriteJSON(w, g.listing(r)) })
	mux.HandleFunc("/messages/", func(w http.ResponseWriter, r *http.Request) {
		message, ok := g.message(r)
		if !ok {
			http.NotFound(w, r)
			return
		}
		integration.WriteJSON(w, message)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// recoveryEnv is a mailbox connected through the production capture registry
// and the real Gmail connector, over the stub.
type recoveryEnv struct {
	e          *integration.SearchEnv
	mailbox    *listedGmail
	registry   *capturemod.Registry
	connection ids.UUID
	owner      context.Context //nolint:containedctx // the mailbox owner's request, reused by every step of one test
	now        *time.Time
}

func newRecoveryEnv(t *testing.T, connected time.Time) recoveryEnv {
	t.Helper()
	e := integration.SetupSearch(t)
	seedCaptureRole(t, e)
	mailbox := &listedGmail{}
	stub := mailbox.serve(t)
	now := connected
	clock := func() time.Time { return now }
	registry := compose.NewCaptureRegistry(e.Pool, newTestKeyvault(t, e), compose.CaptureConfig{Now: clock}).SetClock(clock)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	mail := gmail.New(oauth, gmail.NewAPI(stub.Client(), stub.URL))
	registry.Register(mail)
	authReq, err := gmail.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gmail/callback")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := mail.Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	owner := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	connection, err := registry.Connect(owner, "gmail", auth)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return recoveryEnv{e: e, mailbox: mailbox, registry: registry, connection: connection, owner: owner, now: &now}
}

func (r recoveryEnv) sync(t *testing.T) {
	t.Helper()
	if err := r.registry.SyncOnce(r.owner, r.connection); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
}

// recoverWindow is the owner asking for the last three months of their mailbox
// again, and the worker paging the run to its end.
func (r recoveryEnv) recoverWindow(t *testing.T) capturemod.BackfillRun {
	t.Helper()
	rep := ids.From[ids.UserKind](r.e.Rep1)
	mailbox := capturemod.ProviderMailbox("gmail")
	run, err := r.registry.StartBackfillOver(r.owner, mailbox, rep, 3, connector.BackfillEstimate{}, enqueueNothing)
	if err != nil {
		t.Fatalf("starting the history import: %v", err)
	}
	ws := principal.WithWorkspaceID(context.Background(), r.e.WS)
	for page := 0; ; page++ {
		done, _, _, err := r.registry.RunBackfillStep(ws, run.ID)
		if err != nil {
			t.Fatalf("history import page %d: %v", page, err)
		}
		if done {
			break
		}
		if page > 20 {
			t.Fatal("the history import never finished")
		}
	}
	status, err := r.registry.BackfillStatus(r.owner, mailbox, rep)
	if err != nil || status == nil {
		t.Fatalf("reading the finished import: %+v, %v", status, err)
	}
	return *status
}

// connectionRow is the mailbox as its owner's connection list reads it.
func (r recoveryEnv) connectionRow(t *testing.T) capturemod.ConnectionView {
	t.Helper()
	views, err := r.registry.Connections(r.owner)
	if err != nil || len(views) != 1 {
		t.Fatalf("the owner's connections = %d rows, %v; want the one mailbox", len(views), err)
	}
	return views[0]
}

func (r recoveryEnv) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(r.e.Admin(), r.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// activityOf is the activity a message landed as, or the zero id.
func (r recoveryEnv) activityOf(t *testing.T, note gmailNote) ids.UUID {
	t.Helper()
	var id ids.UUID
	err := database.WithWorkspaceTx(r.e.Admin(), r.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT COALESCE((SELECT id FROM activity WHERE source_id = $1), '00000000-0000-0000-0000-000000000000')`,
			note.id+"@mail.example").Scan(&id)
	})
	if err != nil {
		t.Fatalf("reading the activity of %s: %v", note.id, err)
	}
	return id
}

// captureState is what a replay must leave as it was: each activity's
// audience and restriction, and how many imports, links and audit rows it has.
func (r recoveryEnv) captureState(t *testing.T) map[ids.UUID]string {
	t.Helper()
	state := map[ids.UUID]string{}
	err := database.WithWorkspaceTx(r.e.Admin(), r.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT a.id, concat_ws('|', a.audience, a.restricted_at IS NOT NULL,
			       (SELECT count(*) FROM capture_import i WHERE i.activity_id = a.id),
			       (SELECT count(*) FROM activity_link l WHERE l.activity_id = a.id),
			       (SELECT count(*) FROM audit_log g WHERE g.entity_id = a.id))
			  FROM activity a WHERE a.kind = 'email'`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id ids.UUID
			var shape string
			if err := rows.Scan(&id, &shape); err != nil {
				return err
			}
			state[id] = shape
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatalf("reading the capture state: %v", err)
	}
	return state
}

// customerMet seeds a contact of the owner's at a customer company, and a
// meeting the owner held with them.
func (r recoveryEnv) customerMet(t *testing.T, address string, met time.Time) (contact, meeting ids.UUID) {
	t.Helper()
	company := ids.NewV7()
	contact, meeting = ids.NewV7(), ids.NewV7()
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO contact (id, full_name, owner_id, source, captured_by) VALUES ($1, 'Buyer', $2, 'manual', 'human:x')`, []any{contact, r.e.Rep1}},
		{`INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by) VALUES ($1, $2, true, 'manual', 'human:x')`, []any{contact, address}},
		{`INSERT INTO company (id, display_name, owner_id, status, source, captured_by) VALUES ($1, 'Acme', $2, 'customer', 'manual', 'human:x')`, []any{company, r.e.Rep1}},
		{`INSERT INTO relationship (kind, contact_id, company_id, source, captured_by) VALUES ('employment', $1, $2, 'manual', 'human:x')`, []any{contact, company}},
		{`INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status, host_user_id, source, captured_by)
		  VALUES ($1, 'meeting', 'Conference follow-up', $2, 3600, 'held', $3, 'manual', 'human:x')`, []any{meeting, met, r.e.Rep1}},
		{`INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, []any{meeting, contact}},
	} {
		exec(t, r.e, stmt.sql, stmt.args...)
	}
	return contact, meeting
}

// owesFollowUp is whether Home asks the owner to follow the meeting up.
func (r recoveryEnv) owesFollowUp(t *testing.T, meeting ids.UUID, asOf time.Time) bool {
	t.Helper()
	rows, err := activities.NewStore(r.e.DB()).MeetingsAwaitingFollowUp(ownerReader(r.e), asOf)
	if err != nil {
		t.Fatalf("reading the follow-up reminders: %v", err)
	}
	return slices.ContainsFunc(rows, func(row activities.AwaitingReply) bool { return row.ActivityID == meeting })
}

// ownerReader is the mailbox owner reading their own records, and no one else's.
func ownerReader(e *integration.SearchEnv) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"activity", "contact", "deal", "company", "lead", "relationship"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1}, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeOwn},
	})
}

// newsletters are count inbound messages a minute apart from first.
func newsletters(first time.Time, count int) []gmailNote {
	notes := make([]gmailNote, 0, count)
	for i := range count {
		notes = append(notes, gmailNote{
			id: "digest-" + strconv.Itoa(i), from: "Supplier <ops@supplier.example>", to: recoveryOwner,
			subject: "Order " + strconv.Itoa(i), at: first.Add(time.Duration(i) * time.Minute),
		})
	}
	return notes
}

// The reported shape: the owner met a customer and sent the follow-up from
// Gmail. The mailbox then went quiet longer than Gmail keeps a history cursor.
// More mail arrived since than one recovery round lists. So the sync after the
// stall reads the newest and never the follow-up. The import over the gap files
// it under the contact and settles the reminder. Mail the sync already read
// lands no second time.
func TestAHistoryImportRecoversASentFollowUpTheLostCursorLeftOut(t *testing.T) {
	t.Parallel()
	connected := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	r := newRecoveryEnv(t, connected)
	r.sync(t)

	const buyer = "buyer@acme.example"
	contact, meeting := r.customerMet(t, buyer, connected.Add(25*time.Hour))
	followUp := gmailNote{
		id: "follow-up", from: "Rep <" + recoveryOwner + ">", to: "Buyer <" + buyer + ">",
		subject: "Good to meet you", at: connected.AddDate(0, 0, 2), sent: true,
	}
	private := gmailNote{
		id: "private", from: "Partner <partner@home.example>", to: recoveryOwner,
		subject: "Dinner", at: connected.AddDate(0, 0, 4),
	}
	if _, err := capturemod.NewExclusionStore(r.e.DB()).Add(
		humanWithScopes(r.e, r.e.Rep1, []principal.Scope{principal.ScopeRead, principal.ScopeWrite}),
		capturemod.ExclusionScopeUser, capturemod.ExclusionKindAddress, "partner@home.example", ids.Nil); err != nil {
		t.Fatalf("the owner's exclusion: %v", err)
	}
	stalled := connected.AddDate(0, 0, 13)
	family := gmailNote{
		id: "family", from: "Aunt <aunt@family.example>", to: recoveryOwner,
		subject: "Sunday", at: stalled.Add(time.Hour),
	}
	seedPersonalThread(t, r.e, r.e.Rep1, family.id+"@mail.example", []string{"aunt@family.example"})
	r.mailbox.deliver(append([]gmailNote{followUp, private, family}, newsletters(stalled, 55)...)...)

	resumed := connected.AddDate(0, 0, 14)
	*r.now = resumed
	r.sync(t)

	if got := r.count(t, `SELECT count(*) FROM activity WHERE kind = 'email'`); got != 50 {
		t.Fatalf("the sync after the lost cursor captured %d messages, want the 50 newest Gmail lists", got)
	}
	if !r.activityOf(t, followUp).IsZero() {
		t.Fatal("the follow-up older than the 50 newest was read; the case under test is the one it is not")
	}
	if !r.owesFollowUp(t, meeting, resumed) {
		t.Fatal("before the recovery, Home did not ask for the follow-up the mailbox has not shown it")
	}
	row := r.connectionRow(t)
	if row.LastSuccessAt == nil || !row.LastSuccessAt.Equal(resumed) {
		t.Fatalf("last success = %v, want the sync that just succeeded at %s", row.LastSuccessAt, resumed)
	}
	if row.UnreadFrom == nil || row.UnreadTo == nil ||
		!row.UnreadFrom.Equal(connected) || !row.UnreadTo.Equal(stalled.Add(6*time.Minute)) {
		t.Fatalf("the owner's connection reads unread %v..%v beside a successful sync, want the gap from the last good sync at %s to where the round read back to",
			row.UnreadFrom, row.UnreadTo, connected)
	}
	if audience, _ := audienceOf(t, r.e, r.activityOf(t, family)); audience != "participants" {
		t.Fatalf("mail on a thread judged private reads to %q; the hold under test is not in place", audience)
	}
	before := r.captureState(t)

	run := r.recoverWindow(t)
	if run.Status != "done" || run.Failed != 0 || run.Scanned != 58 {
		t.Fatalf("the import ended %s, scanned %d with %d failed, want done over all 58 with none failed",
			run.Status, run.Scanned, run.Failed)
	}

	recovered := r.activityOf(t, followUp)
	if recovered.IsZero() {
		t.Fatal("the import did not recover the follow-up the sync left out")
	}
	if got := r.count(t, `SELECT count(*) FROM activity WHERE kind = 'email'`); got != 57 {
		t.Errorf("%d messages after the import, want 57: every message once and the excluded one never", got)
	}
	if got := r.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, recovered, contact); got != 1 {
		t.Errorf("the follow-up is linked to its contact %d times, want once", got)
	}
	if got := r.count(t, `SELECT count(*) FROM capture_import WHERE activity_id = $1`, recovered); got != 1 {
		t.Errorf("the follow-up has %d import rows, want one", got)
	}
	timeline, _, err := activities.NewStore(r.e.DB()).ListActivities(ownerReader(r.e), activities.ListActivitiesInput{
		EntityType: new("contact"), EntityID: &contact,
	})
	if err != nil {
		t.Fatalf("reading the contact's history as its owner: %v", err)
	}
	if !slices.ContainsFunc(timeline, func(a crmcontracts.Activity) bool { return ids.UUID(a.Id) == recovered }) {
		t.Errorf("the owner does not see the recovered follow-up in the contact's history (%d rows)", len(timeline))
	}
	if r.owesFollowUp(t, meeting, resumed) {
		t.Error("Home still asks for a follow-up the import recovered")
	}
	if !r.activityOf(t, private).IsZero() {
		t.Error("the import captured mail from an address the owner excluded")
	}
	if audience, _ := audienceOf(t, r.e, r.activityOf(t, family)); audience != "participants" {
		t.Errorf("reading the private thread again opened it to %q", audience)
	}

	after := r.captureState(t)
	for id, shape := range before {
		if after[id] != shape {
			t.Errorf("the import changed mail the sync had already captured: %s became %s", shape, after[id])
		}
	}
	if row := r.connectionRow(t); row.UnreadFrom != nil || row.UnreadTo != nil {
		t.Errorf("the owner's connection reads unread %v..%v after an import that covered it, want none", row.UnreadFrom, row.UnreadTo)
	}
}
