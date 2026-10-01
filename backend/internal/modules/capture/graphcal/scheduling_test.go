// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCalendarInvitationCarriesStableIdentityAndAttendee(t *testing.T) {
	in := connector.CalendarAppointment{CalendarID: "primary", RequestID: "01234567-89ab-cdef-0123-456789abcdef", Subject: "Discovery", Attendees: []string{"guest@example.test"}, Start: time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC)}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/me/calendar/events" || r.Header.Get("Authorization") != "Bearer token" || !strings.Contains(r.Header.Get("Prefer"), "ImmutableId") {
			t.Errorf("unexpected invitation request: %s %s", r.Method, r.URL.Path)
		}
		var event scheduledEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Error(err)
			return
		}
		if event.RequestID != in.RequestID || len(event.Properties) != 1 || event.Properties[0].Value != in.RequestID || len(event.Attendees) != 1 || event.Attendees[0].Address.Address != in.Attendees[0] {
			t.Errorf("invitation lost identity or recipient: %+v", event)
		}
		event.ID = "immutable-event"
		if err := json.NewEncoder(w).Encode(event); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	receipt, err := api.Save(context.Background(), "token", in)
	if err != nil || receipt.EventID != "immutable-event" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestCalendarLookupRefusesAmbiguousOrMismatchedIdentity(t *testing.T) {
	for _, tc := range []struct{ name, body, eventID string }{
		{"ambiguous", `{"value":[{"id":"one"},{"id":"two"}]}`, ""},
		{"mismatch", `{"id":"other"}`, "expected"},
		{"canceled", `{"id":"expected","isCancelled":true}`, "expected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			api := &httpAPI{client: server.Client(), base: server.URL}
			receipt, err := api.Lookup(context.Background(), "token", connector.CalendarAppointment{CalendarID: "primary", EventID: tc.eventID, RequestID: "request"})
			if err == nil || receipt != nil {
				t.Fatalf("accepted ambiguous event: %+v %v", receipt, err)
			}
		})
	}
}

func TestCalendarAvailabilityCountsPrivateAndAllDayEvents(t *testing.T) {
	event := scheduledEvent{ID: "private", AllDay: true, ShowAs: "busy", Start: scheduledTime{"2026-10-04T22:00:00", "UTC"}, End: scheduledTime{"2026-10-05T22:00:00", "UTC"}}
	free := event
	free.ID = "free"
	free.ShowAs = "free"
	declined := event
	declined.ID = "declined"
	declined.Response.Response = "declined"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/calendar/calendarView" || !strings.Contains(r.Header.Get("Prefer"), `outlook.timezone="UTC"`) {
			t.Errorf("wrong calendar: %s", r.URL.Path)
		}
		if err := json.NewEncoder(w).Encode(struct {
			Events []scheduledEvent `json:"value"`
		}{[]scheduledEvent{event, free, declined}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	busy, err := api.Busy(context.Background(), "token", "primary", from, from.Add(24*time.Hour))
	if err != nil || len(busy) != 1 || busy[0].EventID != "private" || busy[0].Start.Hour() != 22 || busy[0].End.Sub(busy[0].Start) != 24*time.Hour {
		t.Fatalf("busy=%+v err=%v", busy, err)
	}
}
