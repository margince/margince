// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	guestColleagueA = "dana@myco.example"
	guestColleagueB = "eli@myco.example"
	guestKnown      = "known@client-co.example"
	guestLater      = "later@client-co.example"
	guestUnread     = "unread@client-co.example"
	guestOptional   = "optional@client-co.example"
	guestAddedLater = "added@client-co.example"
	guestRoom       = "room-4@resource.calendar.google.com"
)

// calendarFeed is a stubbed Google Calendar whose event list a test may change
// between syncs, as an organizer editing the guest list does.
type calendarFeed struct {
	mu    sync.Mutex
	items []map[string]any
}

func (f *calendarFeed) set(items ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = items
}

func (f *calendarFeed) serve(t *testing.T, owner string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"access_token": "access-tok", "refresh_token": "refresh-tok", "expires_in": 3599})
	})
	mux.HandleFunc("/calendars/primary", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"id": owner})
	})
	mux.HandleFunc("/calendars/primary/events", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		integration.WriteJSON(w, map[string]any{"items": f.items, "nextSyncToken": "synctok"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// connectFeed connects the owner's calendar over the production registry and
// answers a sync of it.
func connectFeed(t *testing.T, env captureEnv, feed *calendarFeed) func() {
	t.Helper()
	stub := feed.serve(t, captureOwner)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)
	env.registry.Register(gcal.New(oauth, api))
	authReq, err := gcal.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gcal/callback")
	if err != nil {
		t.Fatal(err)
	}
	granted, err := gcal.New(oauth, api).Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	connID, err := env.registry.Connect(humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead}), "gcal", granted)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return func() {
		t.Helper()
		if err := env.registry.SyncOnce(principal.WithWorkspaceID(context.Background(), env.e.WS), connID); err != nil {
			t.Fatalf("SyncOnce: %v", err)
		}
	}
}

func seatFor(t *testing.T, e *integration.SearchEnv, address string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`, id, address); err != nil {
		t.Fatal(err)
	}
	return id
}

func ensureGuestContact(t *testing.T, e *integration.SearchEnv, address string) ids.ContactID {
	t.Helper()
	contact, err := contacts.NewStore(e.DB()).EnsureContactByEmail(contactCreator(e), "", address, "manual", storekit.RefuseErased)
	if err != nil {
		t.Fatalf("creating the contact %s: %v", address, err)
	}
	return ids.From[ids.ContactKind](contact)
}

// guestList is the invitation the organizer sends: two colleagues, four outside
// guests in every answer state, and a booked room.
func guestList(extra ...map[string]any) []map[string]any {
	optional := guestEntry(guestOptional, "needsAction")
	optional["optional"] = true
	list := []map[string]any{
		ownerEntry("accepted"),
		guestEntry(guestColleagueA, "accepted"),
		guestEntry(guestColleagueB, "tentative"),
		guestEntry(guestKnown, "accepted"),
		guestEntry(guestLater, "declined"),
		guestEntry(guestUnread, "needsAction"),
		optional,
		{"email": guestRoom, "resource": true, "responseStatus": "accepted"},
	}
	return append(list, extra...)
}

type guestRow struct {
	userID, contactID *ids.UUID
}

// guestRows answers each address's participant rows on one meeting, with the
// seat or contact each resolved to.
func guestRows(t *testing.T, e *integration.SearchEnv, meeting ids.UUID) map[string][]guestRow {
	t.Helper()
	out := map[string][]guestRow{}
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT coalesce(ap.address, u.email), ap.user_id, ap.contact_id
			  FROM activity_participant ap LEFT JOIN app_user u ON u.id = ap.user_id
			 WHERE ap.activity_id = $1 AND ap.role IN ('organizer', 'attendee')`, meeting)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var address string
			var row guestRow
			if err := rows.Scan(&address, &row.userID, &row.contactID); err != nil {
				return err
			}
			out[address] = append(out[address], row)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func meetingLinkedTo(t *testing.T, e *integration.SearchEnv, meeting ids.UUID, contact ids.ContactID) bool {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity_link WHERE activity_id = '`+meeting.String()+
		`' AND contact_id = '`+contact.String()+`'`) == 1
}

func gcalMeeting(t *testing.T, e *integration.SearchEnv, sourceID string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM activity WHERE source_system = 'gcal' AND source_id = $1`, sourceID).Scan(&id)
	}); err != nil {
		t.Fatalf("the meeting %s was not captured: %v", sourceID, err)
	}
	return id
}

// Every human on an ordinary invitation is on the meeting, whatever they
// answered and whether or not they are a contact yet. A room is not a guest.
func TestEveryGuestOnAnInvitationIsOnTheMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleagueA, colleagueB := seatFor(t, e, guestColleagueA), seatFor(t, e, guestColleagueB)
	known := ensureGuestContact(t, e, guestKnown)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)

	sync()
	meeting := gcalMeeting(t, e, "review")
	rows := guestRows(t, e, meeting)
	for address, seat := range map[string]ids.UUID{guestColleagueA: colleagueA, guestColleagueB: colleagueB} {
		if len(rows[address]) != 1 || rows[address][0].userID == nil || *rows[address][0].userID != seat {
			t.Errorf("colleague %s: rows %+v, want one naming their seat", address, rows[address])
		}
	}
	for _, address := range []string{guestLater, guestUnread, guestOptional} {
		if len(rows[address]) != 1 || rows[address][0].contactID != nil || rows[address][0].userID != nil {
			t.Errorf("outside guest %s: rows %+v, want one by address", address, rows[address])
		}
	}
	if len(rows[guestKnown]) != 1 || rows[guestKnown][0].contactID == nil || *rows[guestKnown][0].contactID != known.UUID {
		t.Errorf("the guest who is a contact: rows %+v, want one naming the contact", rows[guestKnown])
	}
	if !meetingLinkedTo(t, e, meeting, known) {
		t.Error("the meeting is not filed under the guest who is a contact")
	}
	if len(rows[guestRoom]) != 0 {
		t.Errorf("the booked room is on the meeting as %+v", rows[guestRoom])
	}
	read, err := activities.NewStore(e.DB()).GetActivity(importerCtx(e), ids.From[ids.ActivityKind](meeting), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the meeting: %v", err)
	}
	var shown []string
	for _, guest := range deref(read.Guests) {
		shown = append(shown, guest.Address)
	}
	if want := []string{guestLater, guestOptional, guestUnread}; !slices.Equal(shown, want) {
		t.Errorf("the meeting shows the guests %v, want the outside guests who are not contacts %v", shown, want)
	}

	later := ensureGuestContact(t, e, guestLater)
	store := contacts.NewStore(e.DB())
	if _, err := store.RepairContactCohort(contactCreator(e), later); err != nil {
		t.Fatal(err)
	}
	if !meetingLinkedTo(t, e, meeting, later) {
		t.Error("the meeting is not filed under the guest who became a contact after it")
	}
}

// A guest the organizer adds after the first sync joins the meeting once, and
// a sync that finds no change adds nothing. A guest already on the meeting keeps
// one row, and a link a person moved away stays moved.
func TestAGuestAddedToAnEventJoinsTheMeetingOnce(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seatFor(t, e, guestColleagueA)
	known := ensureGuestContact(t, e, guestKnown)
	feed := &calendarFeed{}
	start := captureNow.AddDate(0, 0, -3)
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)
	sync()
	meeting := gcalMeeting(t, e, "review")
	added := ensureGuestContact(t, e, guestAddedLater)
	ensureGuestContact(t, e, guestUnread)
	elsewhere := ensureGuestContact(t, e, "elsewhere@other-co.example")
	if _, err := activities.NewStore(e.DB()).RelinkActivity(importerCtx(e), ids.From[ids.ActivityKind](meeting),
		activities.RelinkActivityInput{EntityType: "contact", EntityID: elsewhere.UUID, ReplaceExistingOfType: true}); err != nil {
		t.Fatalf("moving the meeting off the known guest: %v", err)
	}

	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList(guestEntry(guestAddedLater, "needsAction"))...))
	sync()
	sync()

	rows := guestRows(t, e, meeting)
	if len(rows[guestAddedLater]) != 1 || rows[guestAddedLater][0].contactID == nil {
		t.Fatalf("the added guest: rows %+v, want one naming their contact", rows[guestAddedLater])
	}
	for _, address := range []string{guestKnown, guestLater, guestUnread, guestOptional} {
		if len(rows[address]) != 1 {
			t.Errorf("guest %s holds %d rows after the update, want 1", address, len(rows[address]))
		}
	}
	if !meetingLinkedTo(t, e, meeting, added) {
		t.Error("the meeting is not filed under the guest added later")
	}
	if meetingLinkedTo(t, e, meeting, known) {
		t.Error("the update filed the meeting again under a guest a person had moved it off")
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+meeting.String()+
		`' AND action = 'update' AND after ? 'guests_added'`); got != 1 {
		t.Errorf("%d audit rows record the added guest, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 1 {
		t.Errorf("%d meetings after the update, want the one", got)
	}
}

// An event that resolves onto a meeting an import filed brings its guests to
// that meeting. The import named one contact; the calendar names everybody.
func TestACalendarEventMatchedOntoAnImportBringsItsGuests(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	colleague := seatFor(t, e, guestColleagueA)
	known := ensureGuestContact(t, e, guestKnown)
	start := captureNow.AddDate(0, 0, -3).Truncate(time.Hour)
	imported := importMeetingFor(t, e, known, "review@google.com", start)
	feed := &calendarFeed{}
	feed.set(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...))
	sync := connectFeed(t, env, feed)

	sync()

	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal'`); got != 0 {
		t.Fatalf("%d meetings under the calendar's own key, want the event matched onto the import", got)
	}
	rows := guestRows(t, e, imported)
	if len(rows[guestColleagueA]) != 1 || rows[guestColleagueA][0].userID == nil || *rows[guestColleagueA][0].userID != colleague {
		t.Errorf("colleague on the imported meeting: rows %+v, want their seat", rows[guestColleagueA])
	}
	for _, address := range []string{guestLater, guestUnread, guestOptional} {
		if len(rows[address]) != 1 {
			t.Errorf("outside guest %s holds %d rows on the imported meeting, want 1", address, len(rows[address]))
		}
	}
	if len(rows[guestRoom]) != 0 {
		t.Errorf("the booked room is on the imported meeting as %+v", rows[guestRoom])
	}
	sync()
	if again := guestRows(t, e, imported); len(again[guestUnread]) != 1 || len(again[guestKnown]) != 1 {
		t.Errorf("a second sync changed the guests: %+v", again)
	}
}

// importMeetingFor files a meeting through the import door, as another CRM's
// export states it: one contact, and the calendar's identity for the meeting.
func importMeetingFor(t *testing.T, e *integration.SearchEnv, contact ids.ContactID, uid string, start time.Time) ids.UUID {
	t.Helper()
	instance := start.UTC().Format(time.RFC3339)
	system, sourceID, subject := "hubspot", "meeting-1", "Quarterly review"
	req := crmcontracts.CreateActivityRequest{
		Kind:         crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
		Subject:      &subject,
		OccurredAt:   &start,
		SourceSystem: &system,
		SourceId:     &sourceID,
		Source:       "hubspot_import:1",
		IcalUid:      &uid,
		IcalInstance: &instance,
	}
	req.Links = &[]struct {
		EntityId   openapi_types.UUID                                `json:"entity_id"`
		EntityType crmcontracts.CreateActivityRequestLinksEntityType `json:"entity_type"`
	}{{EntityId: openapi_types.UUID(contact.UUID), EntityType: "contact"}}
	in, err := activities.LogActivityInputFrom(req)
	if err != nil {
		t.Fatalf("mapping the import: %v", err)
	}
	created, _, err := activities.NewStore(e.DB()).LogActivity(importerCtx(e), in)
	if err != nil {
		t.Fatalf("importing the meeting: %v", err)
	}
	return ids.UUID(created.Id)
}

func importerCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func deref[T any](list *[]T) []T {
	if list == nil {
		return nil
	}
	return *list
}
