// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestInvitationRetryReconcilesTheSameGoogleEvent(t *testing.T) {
	attempts := 0
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer calendar-token" {
			t.Error("missing calendar authorization")
		}
		if r.Method == http.MethodPost {
			attempts++
			var body scheduledEvent
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.ID != "0123456789abcdef0123456789abcdef" || len(body.Attendees) != 1 || body.Attendees[0].Email != "guest@example.test" || r.URL.Query().Get("sendUpdates") != "all" {
				t.Errorf("wrong invitation: %+v query=%s", body, r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusConflict)
			return
		}
		reads++
		if err := json.NewEncoder(w).Encode(scheduledEvent{ID: "0123456789abcdef0123456789abcdef", UID: "event@example.test", Status: "confirmed", Start: eventTime{time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)}, End: eventTime{time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC)}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	receipt, err := api.Save(context.Background(), "calendar-token", connector.CalendarAppointment{CalendarID: "primary", RequestID: "01234567-89ab-cdef-0123-456789abcdef", Attendees: []string{"guest@example.test"}, Start: time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC)})
	if err != nil || receipt.EventID == "" || attempts != 1 || reads != 1 {
		t.Fatalf("receipt=%+v err=%v posts=%d reads=%d", receipt, err, attempts, reads)
	}
}

func TestAvailabilityFailsClosedWhenASelectedGoogleCalendarFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(struct {
			Calendars map[string]busyCalendar `json:"calendars"`
		}{Calendars: map[string]busyCalendar{}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	_, err := api.Busy(context.Background(), "token", "private-calendar", time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("missing selected calendar was reported as free")
	}
}

func TestInvitationConflictDoesNotConfirmADifferentInterval(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if err := json.NewEncoder(w).Encode(scheduledEvent{ID: "0123456789abcdef0123456789abcdef", Status: "confirmed", Start: eventTime{time.Date(2026, 10, 5, 11, 0, 0, 0, time.UTC)}, End: eventTime{time.Date(2026, 10, 5, 11, 30, 0, 0, time.UTC)}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	_, err := api.Save(context.Background(), "token", connector.CalendarAppointment{CalendarID: "primary", RequestID: "01234567-89ab-cdef-0123-456789abcdef", Start: time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC)})
	if err == nil {
		t.Fatal("confirmed an existing event at the wrong time")
	}
}
