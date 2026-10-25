// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A colleague organised a meeting with a contact, and a seat who was a guest
// synced it. It is on the contact's Meetings tab, which reads the contact's
// timeline narrowed to kind=meeting. The event goes through the real gcal
// connector, the calendar mapper and the one Sink.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/gmail"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// gcalStubServing is the stubbed Google Calendar answering with events.
func gcalStubServing(t *testing.T, owner string, events []map[string]any) *httptest.Server {
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
		integration.WriteJSON(w, map[string]any{"items": events, "nextSyncToken": "synctok-1"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// syncGcalEvents connects the stubbed calendar of owner for Rep1 and runs one sync.
func syncGcalEvents(t *testing.T, e *integration.SearchEnv, owner string, events []map[string]any) {
	t.Helper()
	stub := gcalStubServing(t, owner, events)
	oauth := gmail.NewOAuth(gmail.OAuthConfig{ClientID: "cid", ClientSecret: "sec", TokenURL: stub.URL + "/token"})
	api := gcal.NewAPI(stub.Client(), stub.URL)
	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(gcal.New(oauth, api))
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	authReq, err := gcal.AuthRequestFrom("the-code", "https://app.test/v1/connectors/gcal/callback")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := gcal.New(oauth, api).Authenticate(context.Background(), authReq)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	connID, err := registry.Connect(grantCtx, "gcal", auth)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := registry.SyncOnce(grantCtx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}
}

// meetingsTab is what the contact's Meetings tab lists, read as the seat.
func meetingsTab(t *testing.T, e *integration.SearchEnv, contact ids.UUID) []string {
	t.Helper()
	kind, entity := "meeting", "contact"
	page, _, err := activities.NewStore(e.DB()).ListActivities(
		humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead}),
		activities.ListActivitiesInput{Kind: &kind, EntityType: &entity, EntityID: &contact})
	if err != nil {
		t.Fatalf("reading the contact's meetings: %v", err)
	}
	subjects := make([]string, 0, len(page))
	for _, row := range page {
		if row.Subject != nil {
			subjects = append(subjects, *row.Subject)
		}
	}
	return subjects
}

// guestMeetings is a colleague's meeting with a contact a week back, as Google
// lists it to a guest. An internal meeting and a block in the seat's own
// calendar sit beside it.
func guestMeetings(t *testing.T, owner, colleague, guest string) []map[string]any {
	t.Helper()
	weekAgo := clocktest.Now(t).AddDate(0, 0, -7).UTC()
	at := func(d time.Duration) map[string]string {
		return map[string]string{"dateTime": weekAgo.Add(d).Format(time.RFC3339)}
	}
	return []map[string]any{
		{
			"id": "evt-guest", "iCalUID": "guest@google.com", "status": "confirmed", "summary": "Kickoff with Acme",
			"start": at(0), "end": at(time.Hour),
			"organizer": map[string]any{"email": colleague},
			"attendees": []map[string]any{
				{"email": colleague, "organizer": true, "responseStatus": "accepted"},
				{"email": owner, "self": true, "responseStatus": "accepted"},
				{"email": guest, "displayName": "Anna Buyer", "responseStatus": "accepted"},
			},
		},
		{
			"id": "evt-internal", "status": "confirmed", "summary": "Team sync",
			"start": at(2 * time.Hour), "end": at(3 * time.Hour),
			"organizer": map[string]any{"email": colleague},
			"attendees": []map[string]any{{"email": colleague}, {"email": owner, "self": true}},
		},
		{
			"id": "evt-alone", "status": "confirmed", "summary": "Focus time",
			"start": at(4 * time.Hour), "end": at(5 * time.Hour),
			"organizer": map[string]any{"email": owner},
		},
	}
}

func TestAColleaguesMeetingWithAContactIsOnTheirMeetingsTab(t *testing.T) {
	e := integration.SetupSearch(t)
	buyer, err := contacts.NewStore(e.DB()).EnsureContactByEmail(contactCreator(e), "Anna Buyer", "anna@acme.com", "manual", storekit.RefuseErased)
	if err != nil {
		t.Fatalf("seeding the guest: %v", err)
	}
	syncGcalEvents(t, e, "rep@ws.example", guestMeetings(t, "rep@ws.example", "lead@ws.example", "Anna@ACME.com"))

	if listed := meetingsTab(t, e, buyer); len(listed) != 1 || listed[0] != "Kickoff with Acme" {
		t.Fatalf("the guest's Meetings tab lists %q, want only the meeting they were in", listed)
	}
}

// The contact can arrive after the calendar sync, from the invitation mail or
// by hand. The cohort promotion every contact event runs files the meeting then.
func TestAMeetingSyncedBeforeItsGuestBecameAContactReachesTheirMeetingsTab(t *testing.T) {
	e := integration.SetupSearch(t)
	syncGcalEvents(t, e, "rep@ws.example", guestMeetings(t, "rep@ws.example", "lead@ws.example", "anna@acme.com"))
	store := contacts.NewStore(e.DB())
	buyer, err := store.EnsureContactByEmail(contactCreator(e), "Anna Buyer", "anna@acme.com", "manual", storekit.RefuseErased)
	if err != nil {
		t.Fatalf("seeding the guest: %v", err)
	}
	if listed := meetingsTab(t, e, buyer); len(listed) != 0 {
		t.Fatalf("the meeting is filed before the cohort promotion ran (%q), so the case proves nothing", listed)
	}
	promoter := principal.WithCorrelationID(e.Admin(), ids.NewV7())
	if err := database.WithWorkspaceTx(promoter, e.Pool, func(tx pgx.Tx) error {
		_, err := store.PromoteContactCohortTx(promoter, tx, ids.From[ids.ContactKind](buyer))
		return err
	}); err != nil {
		t.Fatalf("promoting the guest's cohort: %v", err)
	}
	if listed := meetingsTab(t, e, buyer); len(listed) != 1 || listed[0] != "Kickoff with Acme" {
		t.Fatalf("the guest's Meetings tab lists %q after the promotion, want only the meeting they were in", listed)
	}
}
