// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The mapping is proven on what the real connector returns, because a test
// that builds its own error cannot notice the connector returning another one.
// Graph names no machine reason in a busy-time answer, so the reason stays
// empty and the cause carries the rest.
func TestAnUnusableGraphAnswerIsAnUnavailableCalendar(t *testing.T) {
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	endsBeforeItStarts := scheduledEvent{ID: "e", ShowAs: "busy", Start: scheduledTime{"2026-10-05T10:00:00", "UTC"}, End: scheduledTime{"2026-10-05T09:00:00", "UTC"}}
	notUTC := scheduledEvent{ID: "e", ShowAs: "busy", Start: scheduledTime{"2026-10-05T10:00:00", "Europe/Berlin"}, End: scheduledTime{"2026-10-05T11:00:00", "Europe/Berlin"}}

	for name, serve := range map[string]func(http.ResponseWriter, *http.Request, string){
		"an interval that ends before it starts": func(w http.ResponseWriter, _ *http.Request, _ string) {
			writeEvents(t, w, "", endsBeforeItStarts)
		},
		"a time that is not UTC": func(w http.ResponseWriter, _ *http.Request, _ string) {
			writeEvents(t, w, "", notUTC)
		},
		"a page chain that never ends": func(w http.ResponseWriter, r *http.Request, base string) {
			writeEvents(t, w, base+r.URL.Path+"?page=again")
		},
		"a body that is not JSON": func(w http.ResponseWriter, _ *http.Request, _ string) {
			if _, err := w.Write([]byte("<html>")); err != nil {
				t.Error(err)
			}
		},
	} {
		var base string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serve(w, r, base) }))
		base = server.URL
		api := &httpAPI{client: server.Client(), base: server.URL}
		_, err := api.Busy(context.Background(), "token", "team@example.test", from, from.Add(24*time.Hour))
		server.Close()
		unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
		if !ok || unavailable.Calendar != "team@example.test" {
			t.Errorf("%s: %v is not an unavailable calendar", name, err)
		}
	}
}

// A refused credential and an unreachable provider keep the classes the caller
// already acts on.
func TestAGraphCredentialRefusalAndAnOutageKeepTheirOwnClass(t *testing.T) {
	from := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for status, want := range map[int]error{
		http.StatusUnauthorized:       connector.ErrAuthRejected,
		http.StatusServiceUnavailable: connector.ErrUnreachable,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
		api := &httpAPI{client: server.Client(), base: server.URL}
		_, err := api.Busy(context.Background(), "token", "primary", from, from.Add(24*time.Hour))
		server.Close()
		if !errors.Is(err, want) {
			t.Errorf("status %d → %v, want %v", status, err, want)
		}
		if _, rewritten := errors.AsType[*connector.CalendarUnavailableError](err); rewritten {
			t.Errorf("status %d was reported as an unreadable calendar", status)
		}
	}
}

func writeEvents(t *testing.T, w http.ResponseWriter, next string, events ...scheduledEvent) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(struct {
		Events []scheduledEvent `json:"value"`
		Next   string           `json:"@odata.nextLink,omitempty"`
	}{events, next}); err != nil {
		t.Error(err)
	}
}
