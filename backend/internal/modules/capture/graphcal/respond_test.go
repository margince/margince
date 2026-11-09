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

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestEachAnswerPostsMicrosoftsOwnAction(t *testing.T) {
	t.Parallel()
	for answer, action := range map[string]string{
		"accepted": "accept", "tentative": "tentativelyAccept", "declined": "decline",
	} {
		t.Run(answer, func(t *testing.T) {
			t.Parallel()
			var posted string
			var body map[string]bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if got := r.URL.Query().Get("$filter"); got != "iCalUId eq 'o''brien-040000008200'" {
						t.Errorf("filter = %q, want the UID quoted for OData", got)
					}
					_, _ = w.Write([]byte(`{"value":[{"id":"AAMk1"}]}`))
					return
				}
				posted = r.URL.Path
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode: %v", err)
				}
				w.WriteHeader(http.StatusAccepted)
			}))
			defer server.Close()
			api := &httpAPI{client: server.Client(), base: server.URL}
			if err := api.Respond(context.Background(), "token", "o'brien-040000008200", answer); err != nil {
				t.Fatalf("Respond: %v", err)
			}
			if posted != "/me/events/AAMk1/"+action {
				t.Errorf("posted %q, want the %s action", posted, action)
			}
			if !body["sendResponse"] {
				t.Error("the organizer must be told: sendResponse is false")
			}
		})
	}
}

func TestAnEventMissingFromTheCalendarIsNotAnswered(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("wrote %s to a calendar that does not hold the invitation", r.Method)
		}
		_, _ = w.Write([]byte(`{"value":[]}`))
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	err := api.Respond(context.Background(), "token", "uid", "accepted")
	if !errors.Is(err, connector.ErrCalendarEventMissing) {
		t.Errorf("Respond = %v, want ErrCalendarEventMissing", err)
	}
}
