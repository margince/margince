// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const teamCalendar = "team@example.test"

// serveGoogle answers freeBusy with an empty, error-free calendar. The event
// read goes to events, so a test drives the shape of one answer at a time.
func serveGoogle(t *testing.T, events func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/freeBusy") {
			answer := map[string]map[string]busyCalendar{"calendars": {teamCalendar: {}}}
			if err := json.NewEncoder(w).Encode(answer); err != nil {
				t.Error(err)
			}
			return
		}
		events(w, r)
	}))
}

// The mapping is proven on what the real connector returns, because a test
// that builds its own error cannot notice the connector returning another one.
func TestAnUnusableGoogleEventAnswerIsAnUnavailableCalendar(t *testing.T) {
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for name, events := range map[string]func(http.ResponseWriter, *http.Request){
		"an interval that ends before it starts": func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte(`{"timeZone":"UTC","items":[{"id":"e","start":{"dateTime":"2026-10-05T10:00:00Z"},"end":{"dateTime":"2026-10-05T09:00:00Z"}}]}`)); err != nil {
				t.Error(err)
			}
		},
		"a time zone nobody can load": func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte(`{"timeZone":"Nowhere/Atlantis","items":[]}`)); err != nil {
				t.Error(err)
			}
		},
		"a page chain that never ends": func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte(`{"timeZone":"UTC","nextPageToken":"again","items":[]}`)); err != nil {
				t.Error(err)
			}
		},
		"a body that is not JSON": func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte("<html>")); err != nil {
				t.Error(err)
			}
		},
	} {
		server := serveGoogle(t, events)
		api := &httpAPI{client: server.Client(), base: server.URL}
		_, err := api.Busy(context.Background(), "token", teamCalendar, from, from.Add(24*time.Hour))
		server.Close()
		unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
		if !ok || unavailable.Calendar != teamCalendar {
			t.Errorf("%s: %v is not an unavailable calendar", name, err)
		}
	}
}

// A refused credential and an unreachable provider keep the classes the caller
// already acts on.
func TestAGoogleCredentialRefusalAndAnOutageKeepTheirOwnClass(t *testing.T) {
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for status, want := range map[int]error{
		http.StatusUnauthorized:       connector.ErrAuthRejected,
		http.StatusServiceUnavailable: connector.ErrUnreachable,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		api := &httpAPI{client: server.Client(), base: server.URL}
		_, err := api.Busy(context.Background(), "token", teamCalendar, from, from.Add(24*time.Hour))
		server.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d → %v, want %v", status, err, want)
		}
		if _, rewritten := errors.AsType[*connector.CalendarUnavailableError](err); rewritten {
			t.Errorf("status %d was reported as an unreadable calendar", status)
		}
	}
}
