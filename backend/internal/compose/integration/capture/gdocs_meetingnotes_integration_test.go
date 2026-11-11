// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gdocs"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const notesText = "Summary\nAda will send the offer by Friday.\n"

// docsFeed is a stubbed Google Docs API. Each Doc answers its text, or the
// error a test sets for it, and every read is counted.
type docsFeed struct {
	mu      sync.Mutex
	refused bool
	reads   []string
}

func (f *docsFeed) readCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reads)
}

func (f *docsFeed) serve(t *testing.T, account string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{
			"access_token": "docs-access", "refresh_token": "docs-refresh", "expires_in": 3599,
			"scope": strings.Join(gdocs.ConsentScopes(), " "),
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"email": account})
	})
	mux.HandleFunc("/v1/documents/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/documents/")
		f.mu.Lock()
		f.reads = append(f.reads, id)
		refused := f.refused
		f.mu.Unlock()
		if refused {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			integration.WriteJSON(w, map[string]any{"error": map[string]any{
				"code": 403, "status": "PERMISSION_DENIED",
				"details": []any{map[string]any{"reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT"}},
			}})
			return
		}
		integration.WriteJSON(w, map[string]any{
			"title": "Kick-off review - Notes by Gemini", "revisionId": "rev-" + id,
			"tabs": []any{map[string]any{"documentTab": map[string]any{"body": map[string]any{"content": []any{
				map[string]any{"paragraph": map[string]any{"elements": []any{
					map[string]any{"textRun": map[string]any{"content": notesText}},
				}}},
			}}}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// queuedReadings counts the readings the landing queued.
type queuedReadings struct {
	mu       sync.Mutex
	activity []ids.ActivityID
}

func (q *queuedReadings) enqueue(_ context.Context, _ pgx.Tx, read activities.TranscriptRead) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.activity = append(q.activity, read.ActivityID)
	return nil
}

// connectNotes connects Rep1's Google Docs grant on account, over a registry
// built the way production builds it, and answers a sync of it.
func connectNotes(t *testing.T, env captureEnv, feed *docsFeed, account string, queued *queuedReadings) (ids.UUID, func()) {
	t.Helper()
	stub := feed.serve(t, account)
	oauth := gdocs.NewOAuth(gdocs.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	conn := gdocs.New(oauth, gdocs.NewOwner(stub.Client(), stub.URL)).WithDocs(gdocs.NewDocsAPI(stub.Client(), stub.URL))
	reg := compose.NewCaptureRegistry(env.e.Pool, newTestKeyvault(t, env.e), compose.CaptureConfig{
		Now: func() time.Time { return captureNow }, TranscriptEnqueue: queued.enqueue,
	})
	reg.Register(conn)
	authReq, err := gdocs.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gdocs/callback")
	if err != nil {
		t.Fatal(err)
	}
	granted, err := conn.Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	connID, err := reg.Connect(humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead}), "gdocs", granted)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return connID, func() {
		t.Helper()
		if err := reg.SyncOnce(principal.WithWorkspaceID(context.Background(), env.e.WS), connID); err != nil {
			t.Fatalf("SyncOnce gdocs: %v", err)
		}
	}
}

// meetingWithNotes syncs Rep1's calendar once with a Meet call carrying its
// notes Doc, and answers the meeting and the feed to change it.
func meetingWithNotes(t *testing.T, env captureEnv) (ids.UUID, *calendarFeed, func()) {
	t.Helper()
	seatFor(t, env.e, guestColleagueA)
	ensureGuestContact(t, env.e, guestKnown)
	feed := &calendarFeed{}
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...),
		driveFile(notesFileID, "Kick-off review - Notes by Gemini")))
	sync := connectFeed(t, env, feed)
	sync()
	return gcalMeeting(t, env.e), feed, sync
}

func notesURL(fileID string) string {
	return "https://docs.google.com/document/d/" + fileID + "/edit"
}

// transcriptsOf counts the transcripts landed from one Doc.
func transcriptsOf(t *testing.T, e *integration.SearchEnv, fileID string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'transcript' AND source_id = '`+notesURL(fileID)+`'`)
}

// A meeting's notes Doc is read through the owner's Docs grant. It lands once,
// as a transcript of that meeting, and the activity writer queues its reading.
// A second sync opens no Doc again.
func TestMeetingNotesLandOnceAsATranscriptAndQueueAReading(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, _, _ := meetingWithNotes(t, env)
	docs, queued := &docsFeed{}, &queuedReadings{}
	_, syncNotes := connectNotes(t, env, docs, captureOwner, queued)

	syncNotes()
	syncNotes()

	if got := docs.readCount(); got != 1 {
		t.Errorf("the notes Doc was opened %d times over two syncs, want once", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 1 {
		t.Fatalf("%d transcripts from the notes Doc, want 1", got)
	}
	var (
		transcript ids.ActivityID
		body, kind string
		from       string
		capturedBy string
	)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id, body, kind, raw -> 'google_doc' ->> 'meeting_id', captured_by FROM activity
			 WHERE source_system = 'transcript' AND source_id = $1`, notesURL(notesFileID)).
			Scan(&transcript, &body, &kind, &from, &capturedBy)
	}); err != nil {
		t.Fatal(err)
	}
	if body != strings.TrimSuffix(notesText, "\n") || kind != "meeting" || from != meeting.String() {
		t.Errorf("transcript: body %q kind %q from %q, want the notes text on a meeting read from %s", body, kind, from, meeting)
	}
	if !strings.HasPrefix(capturedBy, "connector:") {
		t.Errorf("transcript captured_by = %q, want the connector principal", capturedBy)
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+transcript.String()+`' AND action = 'create'`); got != 1 {
		t.Errorf("%d create audits on the transcript, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = '`+transcript.String()+`'`); got != 1 {
		t.Errorf("%d events announced the transcript, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM transcript_read WHERE activity_id = '`+transcript.String()+`' AND status = 'queued'`); got != 1 {
		t.Errorf("%d queued readings of the transcript, want 1", got)
	}
	if !slices.Equal(queued.activity, []ids.ActivityID{transcript}) {
		t.Errorf("readings enqueued for %v, want the transcript once", queued.activity)
	}
}

// A Doc taken off the event is archived from the meeting at the next calendar
// sync, and the notes reader never opens it.
func TestANotesDocTakenOffTheEventIsNotRead(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, feed, syncCalendar := meetingWithNotes(t, env)
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...)))
	syncCalendar()

	if got := countRows(t, e, `SELECT count(*) FROM attachment WHERE entity_id = '`+meeting.String()+
		`' AND category = 'link' AND archived_at IS NOT NULL`); got != 1 {
		t.Fatalf("%d archived notes links on the meeting, want the removed Doc", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+meeting.String()+
		`' AND (after ->> 'meeting_documents_removed')::int = 1`); got != 1 {
		t.Errorf("%d meeting audits record the removed Doc, want 1", got)
	}

	docs := &docsFeed{}
	_, syncNotes := connectNotes(t, env, docs, captureOwner, &queuedReadings{})
	syncNotes()

	if got := docs.readCount(); got != 0 {
		t.Errorf("a Doc taken off the event was opened %d times, want never", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts from a removed Doc, want 0", got)
	}
}

// A Docs grant on another Google account than the calendar's reads nothing:
// notes are read only for the meetings of the account that granted them.
func TestANotesGrantOnAnotherAccountReadsNothing(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meetingWithNotes(t, env)
	docs := &docsFeed{}
	_, syncNotes := connectNotes(t, env, docs, "owner.private@myco.example", &queuedReadings{})
	syncNotes()

	if got := docs.readCount(); got != 0 {
		t.Errorf("a grant on another account opened %d Docs, want none", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts through another account, want 0", got)
	}
}

// Google refusing the read grant takes it off the connection's recorded
// scopes, which is what Connections reads to say the row cannot read notes.
// The next sync opens nothing until the owner connects again.
func TestARefusedNotesGrantAsksTheOwnerToReconnect(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meetingWithNotes(t, env)
	docs := &docsFeed{refused: true}
	connID, syncNotes := connectNotes(t, env, docs, captureOwner, &queuedReadings{})
	syncNotes()

	var scopes []string
	var status string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT provider_scopes, status FROM capture_connection WHERE id = $1`, connID).Scan(&scopes, &status)
	}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(scopes, gdocs.ReadScope) {
		t.Errorf("recorded scopes %v still hold the refused read grant", scopes)
	}
	if !slices.Contains(scopes, gdocs.EmailScope) {
		t.Errorf("recorded scopes %v lost a grant Google did not refuse", scopes)
	}
	if status != "connected" {
		t.Errorf("connection status %q, want connected: the account still syncs", status)
	}
	syncNotes()
	if got := docs.readCount(); got != 1 {
		t.Errorf("%d Doc reads after the refusal, want only the refused one", got)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts after a refused grant, want 0", got)
	}
}

// The landing asks the meeting's evidence again under its own lock. Text read
// before a Doc was taken off the event does not land. Neither does text when
// the organizer's domain is not among the trusted domains handed in.
func TestNotesLandOnlyOnEvidenceStillTrueAtTheWrite(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, feed, syncCalendar := meetingWithNotes(t, env)
	store := activities.NewStore(e.DB())
	land := func(domain string) bool {
		t.Helper()
		landed, err := landAs(e, store, e.Rep1, meeting, domain)
		if err != nil {
			t.Fatalf("LandMeetingNotesTx: %v", err)
		}
		return landed
	}
	if land("client-co.example") {
		t.Error("notes landed for an owner whose domain did not organize the meeting")
	}
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...)))
	syncCalendar()
	if land("myco.example") {
		t.Error("notes landed after the Doc was taken off the event")
	}
	if got := transcriptsOf(t, e, notesFileID); got != 0 {
		t.Errorf("%d transcripts, want 0", got)
	}
}

// landAs lands the notes Doc of meeting for user, through the store, in its
// own transaction.
func landAs(e *integration.SearchEnv, store *activities.Store, user ids.UUID, meeting ids.UUID, domain string) (bool, error) {
	var landed bool
	ctx := humanWithScopes(e, user, []principal.Scope{principal.ScopeRead})
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		landed, err = store.LandMeetingNotesTx(ctx, tx, landing(user, meeting, domain))
		return err
	})
	return landed, err
}

func landing(user, meeting ids.UUID, domain string) activities.MeetingNotesInput {
	return activities.MeetingNotesInput{
		Meeting: ids.From[ids.ActivityKind](meeting), URL: notesURL(notesFileID),
		Owner: ids.From[ids.UserKind](user), Domains: []string{domain}, Text: notesText,
	}
}

// An outsider on a shared webmail domain organizes a Meet call with a seat
// whose calendar is on that same domain. Webmail is nobody's own domain, so
// the Doc is not recorded on the meeting and nothing is read.
func TestAnOutsiderOnTheSeatsWebmailDomainCannotPlantNotes(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	const seat, outsider = "ada.rep@gmail.com", "mallory@gmail.com"
	feed := &calendarFeed{}
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), outsider,
		selfEntry(seat, "accepted"), guestEntry(outsider, "accepted")),
		driveFile(notesFileID, "Our call - Notes by Gemini")))
	connectFeedAs(t, env, feed, seat)()
	docs := &docsFeed{}
	_, syncNotes := connectNotes(t, env, docs, seat, &queuedReadings{})
	syncNotes()

	if got := notesRows(t, e, gcalMeeting(t, e).String()); got != 0 {
		t.Errorf("%d notes links from an outsider on webmail, want 0", got)
	}
	if got := docs.readCount(); got != 0 || transcriptsOf(t, e, notesFileID) != 0 {
		t.Errorf("an outsider's Doc was opened %d times, want never and no transcript", got)
	}
}

// The company stops claiming the organizer's domain after the Doc was
// recorded. The reader asks the trusted domains again and opens nothing.
func TestNotesAreNotReadOnceTheOrganizersDomainIsNoLongerOurs(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meetingWithNotes(t, env)
	if _, err := e.Owner.Exec(context.Background(), `DELETE FROM company_domain WHERE domain = 'myco.example'`); err != nil {
		t.Fatal(err)
	}
	docs := &docsFeed{}
	_, syncNotes := connectNotes(t, env, docs, captureOwner, &queuedReadings{})
	syncNotes()

	if got := docs.readCount(); got != 0 || transcriptsOf(t, e, notesFileID) != 0 {
		t.Errorf("a Doc of a no-longer-trusted organizer was opened %d times, want never", got)
	}
}

// readsActivity reports whether user may read the activity's content.
func readsActivity(t *testing.T, e *integration.SearchEnv, user, activity ids.UUID) bool {
	t.Helper()
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	_, err := activities.NewStore(e.DB()).GetActivityContent(ctx, ids.From[ids.ActivityKind](activity), storekit.LiveOnly)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading %s as %s: %v", activity, user, err)
	}
	return err == nil
}

// On a meeting limited to its participants the transcript takes the same
// audience, whichever seat read the Doc. A colleague on the invitation reads
// it; a colleague who was not invited does not.
func TestATranscriptIsReadByWhoeverReadsItsMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, _, _ := meetingWithNotes(t, env)
	attendee := seatOf(t, e, guestColleagueA)
	stranger := seatFor(t, e, "not.invited@myco.example")
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE activity SET audience = 'participants' WHERE id = $1`, meeting); err != nil {
		t.Fatal(err)
	}
	if !readsActivity(t, e, attendee, meeting) || readsActivity(t, e, stranger, meeting) {
		t.Fatal("the fixture meeting is not limited to its participants")
	}
	_, syncNotes := connectNotes(t, env, &docsFeed{}, captureOwner, &queuedReadings{})
	syncNotes()

	transcript := transcriptID(t, e, notesFileID)
	if !readsActivity(t, e, e.Rep1, transcript) {
		t.Error("the seat that read the Doc cannot read its transcript")
	}
	if !readsActivity(t, e, attendee, transcript) {
		t.Error("a colleague on the meeting cannot read its transcript")
	}
	if readsActivity(t, e, stranger, transcript) {
		t.Error("a colleague not on the meeting reads its transcript")
	}
}

func seatOf(t *testing.T, e *integration.SearchEnv, address string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.Owner.QueryRow(context.Background(), `SELECT id FROM app_user WHERE email = $1`, address).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func transcriptID(t *testing.T, e *integration.SearchEnv, fileID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_system = 'transcript' AND source_id = $1`, notesURL(fileID)).Scan(&id)
	}); err != nil {
		t.Fatalf("no transcript of %s: %v", fileID, err)
	}
	return id
}

// A Doc taken off the event and attached again is live on the meeting again,
// and the notes reader opens it.
func TestANotesDocAttachedAgainIsReadAgain(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, feed, syncCalendar := meetingWithNotes(t, env)
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...)))
	syncCalendar()
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...),
		driveFile(notesFileID, "Kick-off review - Notes by Gemini")))
	syncCalendar()

	if got := countRows(t, e, `SELECT count(*) FROM attachment WHERE entity_id = '`+meeting.String()+
		`' AND category = 'link' AND archived_at IS NULL`); got != 1 {
		t.Fatalf("%d live notes links after the Doc came back, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_type = 'attachment' AND action = 'restore'`); got != 1 {
		t.Errorf("%d restore audits, want 1", got)
	}
	_, syncNotes := connectNotes(t, env, &docsFeed{}, captureOwner, &queuedReadings{})
	syncNotes()
	if got := transcriptsOf(t, e, notesFileID); got != 1 {
		t.Errorf("%d transcripts from the Doc attached again, want 1", got)
	}
}

// Two colleagues' syncs land one Doc at once. The second waits for the first
// and finds its transcript, rather than failing on the transcript's key.
func TestTwoSyncsLandingOneDocAtOnceLandItOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	meeting, _, _ := meetingWithNotes(t, env)
	store := activities.NewStore(e.DB())
	ctx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	firstLanded, release, firstDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		firstDone <- database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			landed, err := store.LandMeetingNotesTx(ctx, tx, landing(e.Rep1, meeting, "myco.example"))
			if err != nil || !landed {
				return errors.Join(err, errors.New("the first sync did not land the notes"))
			}
			close(firstLanded)
			<-release
			return nil
		})
	}()
	select {
	case <-firstLanded:
	case err := <-firstDone:
		t.Fatalf("first landing: %v", err)
	}
	type outcome struct {
		landed bool
		err    error
	}
	secondDone := make(chan outcome, 1)
	go func() {
		landed, err := landAs(e, store, e.Rep1, meeting, "myco.example")
		secondDone <- outcome{landed, err}
	}()
	waitForALockWait(t, e)
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first landing: %v", err)
	}
	second := <-secondDone
	if second.err != nil || second.landed {
		t.Errorf("second landing: landed %v, err %v; want it to find the first one's transcript", second.landed, second.err)
	}
	if got := transcriptsOf(t, e, notesFileID); got != 1 {
		t.Errorf("%d transcripts, want 1", got)
	}
}

// waitForALockWait returns once some session of this database waits on a
// lock, which is the second landing blocked behind the first.
func waitForALockWait(t *testing.T, e *integration.SearchEnv) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		var waiting int
		if err := e.Owner.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting); err != nil {
			t.Fatalf("no session ever waited on a lock: %v", err)
		}
		if waiting > 0 {
			return
		}
	}
}
