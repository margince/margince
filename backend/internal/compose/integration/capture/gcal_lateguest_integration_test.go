// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The colleague who organised the meeting: a seat on the owner's domain that
// never connected anything.
const lateGuestOrganizer = "colleague@myco.example"

// The three guests from the outside company, none of them a contact yet.
var lateGuests = []string{"ana@outside-co.example", "ben@outside-co.example", "cem@outside-co.example"}

// gcalEventsStub answers the Calendar endpoints with the given event resources
// on every list call.
func gcalEventsStub(t *testing.T, owner string, items []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		body := map[string]any{"access_token": "access-tok", "expires_in": 3599}
		if r.Form.Get("grant_type") == "authorization_code" {
			body["refresh_token"] = "refresh-tok"
		}
		integration.WriteJSON(w, body)
	})
	mux.HandleFunc("/calendars/primary", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"id": owner})
	})
	mux.HandleFunc("/calendars/primary/events", func(w http.ResponseWriter, _ *http.Request) {
		integration.WriteJSON(w, map[string]any{"items": items, "nextSyncToken": "synctok-1"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// lateGuestEvent is one event resource as Google lists it on the owner's
// primary calendar, starting at start.
func lateGuestEvent(id, status string, start time.Time, organizer string, attendees ...map[string]any) map[string]any {
	return map[string]any{
		"id": id, "iCalUID": id + "@google.com", "status": status, "summary": "Kick-off " + id,
		"start":     map[string]string{"dateTime": start.Format(time.RFC3339)},
		"end":       map[string]string{"dateTime": start.Add(time.Hour).Format(time.RFC3339)},
		"organizer": map[string]string{"email": organizer},
		"attendees": attendees,
	}
}

func guestEntry(email, response string) map[string]any {
	return map[string]any{"email": email, "responseStatus": response}
}

func ownerEntry(response string) map[string]any {
	return selfEntry(captureOwner, response)
}

// selfEntry is the calendar owner's own attendee entry, as Google marks it.
func selfEntry(owner, response string) map[string]any {
	return map[string]any{"email": owner, "self": true, "responseStatus": response}
}

// connectStubbedCalendar connects the owner's stubbed Google Calendar on the
// env's production registry and answers the connection.
func connectStubbedCalendar(t *testing.T, env captureEnv, items []map[string]any) ids.UUID {
	t.Helper()
	return connectCalendarOf(t, env, captureOwner, items)
}

// connectCalendarOf is connectStubbedCalendar for a calendar on another
// address than the env's mailbox.
func connectCalendarOf(t *testing.T, env captureEnv, owner string, items []map[string]any) ids.UUID {
	t.Helper()
	stub := gcalEventsStub(t, owner, items)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)
	env.registry.Register(gcal.New(oauth, api))

	authReq, err := gcal.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gcal/callback")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := gcal.New(oauth, api).Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	connID, err := env.registry.Connect(humanWithScopes(env.e, env.e.Rep1, []principal.Scope{principal.ScopeRead}), "gcal", auth)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	return connID
}

// colleagueInvitation is the mail Google sends when a colleague invites the
// owner and the guests. From is the organiser, and the calendar part says
// REQUEST.
func colleagueInvitation(msgID string) []byte {
	return []byte(strings.Join([]string{
		"Sender: Google Calendar <calendar-notification@google.com>",
		"From: " + lateGuestOrganizer,
		"To: " + captureOwner + ", " + strings.Join(lateGuests, ", "),
		"Subject: Invitation: Kick-off @ Wed 20 May 2026 10am",
		"Date: Tue, 19 May 2026 08:00:00 +0000",
		"Message-ID: <" + msgID + ">",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"You have been invited to Kick-off.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	}, "\r\n"))
}

// A colleague organised a past meeting with outside guests, and the owner
// accepted it. It is captured from the owner's calendar, and it is filed on each
// guest once the guest is a contact. The invitation mail stays its own row.
// A solo block, a meeting within the owner's domain, a declined meeting and a
// cancelled one are never captured.
func TestAColleaguesMeetingWithOutsideGuestsIsFiledOnGuestsWhoArriveLater(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	held := captureNow.AddDate(0, 0, -16)
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		ids.NewV7(), lateGuestOrganizer); err != nil {
		t.Fatal(err)
	}

	env.sync(t, colleagueInvitation("invite-kickoff@google.com"))

	guests := []map[string]any{ownerEntry("accepted"), guestEntry(lateGuestOrganizer, "accepted")}
	for _, g := range lateGuests {
		guests = append(guests, guestEntry(g, "needsAction"))
	}
	connID := connectStubbedCalendar(t, env, []map[string]any{
		lateGuestEvent("kickoff", "confirmed", held, lateGuestOrganizer, guests...),
		lateGuestEvent("focus", "confirmed", held, captureOwner, ownerEntry("accepted")),
		lateGuestEvent("standup", "confirmed", held, lateGuestOrganizer,
			ownerEntry("accepted"), guestEntry(lateGuestOrganizer, "accepted")),
		lateGuestEvent("declined", "confirmed", held, lateGuestOrganizer,
			ownerEntry("declined"), guestEntry(lateGuests[0], "accepted")),
		lateGuestEvent("cancelled", "cancelled", held, lateGuestOrganizer,
			ownerEntry("accepted"), guestEntry(lateGuests[0], "accepted")),
	})
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := env.registry.SyncOnce(wsCtx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'gcal' AND source_id <> 'kickoff'`); got != 0 {
		t.Fatalf("%d meetings captured from the solo, internal, declined and cancelled events, want 0", got)
	}
	var meeting ids.UUID
	var occurred time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT id, occurred_at FROM activity
			 WHERE source_system = 'gcal' AND source_id = 'kickoff' AND kind = 'meeting'`).Scan(&meeting, &occurred)
	}); err != nil {
		t.Fatalf("the colleague's meeting with outside guests was not captured: %v", err)
	}
	if !occurred.Equal(held) {
		t.Errorf("meeting dated %s, want the event's own start %s", occurred, held)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity WHERE kind = 'email' AND source_id = 'invite-kickoff@google.com'`); got != 1 {
		t.Errorf("%d rows for the invitation mail, want 1 beside the meeting", got)
	}

	store := contacts.NewStore(e.DB())
	ctx := contactCreator(e)
	var arrived []ids.ContactID
	for _, g := range lateGuests {
		contact, err := store.EnsureContactByEmail(ctx, "", g, "manual")
		if err != nil {
			t.Fatalf("creating the guest %s: %v", g, err)
		}
		arrived = append(arrived, ids.From[ids.ContactKind](contact))
	}
	owed, err := store.ContactsOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(owed) != len(arrived) {
		t.Fatalf("the sweep offers %d contacts, want the %d guests who arrived after the meeting", len(owed), len(arrived))
	}
	for _, contact := range owed {
		if _, err := store.RepairContactCohort(ctx, contact); err != nil {
			t.Fatalf("repairing %s: %v", contact, err)
		}
		again, err := store.RepairContactCohort(ctx, contact)
		if err != nil || again.Linked != 0 || again.Promoted != 0 {
			t.Fatalf("a second repair of %s changed %+v (err %v), want nothing", contact, again, err)
		}
	}
	if got := countRows(t, e, `
		SELECT count(*) FROM activity_link l JOIN contact_email ce ON ce.contact_id = l.contact_id
		 WHERE l.activity_id = '`+meeting.String()+`' AND ce.email LIKE '%@outside-co.example'`); got != len(lateGuests) {
		t.Fatalf("meeting filed on %d guests, want all %d once they are contacts", got, len(lateGuests))
	}
}
