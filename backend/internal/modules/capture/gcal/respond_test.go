// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAnAnswerChangesOnlyTheOwnersResponseAndTellsTheGuests(t *testing.T) {
	t.Parallel()
	var patched map[string][]map[string]any
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("iCalUID") != "kickoff@google.com" {
				t.Errorf("looked the event up by %q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"ev1","attendees":[
				{"email":"organizer@partner.example","organizer":true,"responseStatus":"accepted"},
				{"email":"owner@myco.com","self":true,"responseStatus":"needsAction","comment":"kept"}]}]}`))
		case http.MethodPatch:
			query = r.URL.Path + "?" + r.URL.RawQuery
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				t.Errorf("decode patch: %v", err)
			}
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	if err := api.Respond(context.Background(), "token", "kickoff@google.com", "tentative"); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if query != "/calendars/primary/events/ev1?sendUpdates=all" {
		t.Errorf("patched %q, want the event with sendUpdates=all", query)
	}
	guests := patched["attendees"]
	if len(guests) != 2 || guests[0]["responseStatus"] != "accepted" {
		t.Fatalf("the organizer's own answer must go back unchanged, got %v", guests)
	}
	if guests[1]["responseStatus"] != "tentative" || guests[1]["comment"] != "kept" {
		t.Errorf("the owner's row = %v, want tentative with its other fields kept", guests[1])
	}
}

func TestAnEventThatDoesNotListTheOwnerIsNotAnswered(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"no such event":        `{"items":[]}`,
		"owner is not a guest": `{"items":[{"id":"ev1","attendees":[{"email":"someone@else.example"}]}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("wrote %s to a calendar that does not hold the invitation", r.Method)
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			api := &httpAPI{client: server.Client(), base: server.URL}
			err := api.Respond(context.Background(), "token", "kickoff@google.com", "accepted")
			if !errors.Is(err, connector.ErrCalendarEventMissing) {
				t.Errorf("Respond = %v, want ErrCalendarEventMissing", err)
			}
		})
	}
}

// answeringAPI is the provider API with the answer verb, recording what the
// connector passed it.
type answeringAPI struct {
	fakeAPI
	token, uid, response string
}

func (a *answeringAPI) Respond(_ context.Context, token, uid, response string) error {
	a.token, a.uid, a.response = token, uid, response
	return nil
}

func TestTheConnectorAnswersWithTheOwnersFreshToken(t *testing.T) {
	api := &answeringAPI{}
	c := New(fakeOAuth{access: "access-1"}, api)
	if err := c.CalendarRespond(context.Background(), authBytes(t), "kickoff@example.com", connector.ResponseDeclined); err != nil {
		t.Fatalf("CalendarRespond: %v", err)
	}
	if api.token != "access-1" || api.uid != "kickoff@example.com" || api.response != connector.ResponseDeclined {
		t.Errorf("the API was asked %+v, want the declined answer with the refreshed token", api)
	}
}

func TestAnAPIThatCannotAnswerIsUnreachable(t *testing.T) {
	c := New(fakeOAuth{access: "access-1"}, &fakeAPI{})
	err := c.CalendarRespond(context.Background(), authBytes(t), "kickoff@example.com", connector.ResponseAccepted)
	if !errors.Is(err, connector.ErrUnreachable) {
		t.Errorf("CalendarRespond = %v, want ErrUnreachable", err)
	}
}
