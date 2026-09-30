// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const teamsLink = "https://teams.microsoft.com/l/meetup-join/meeting"

func TestOutlookIsAskedForAnOnlineMeetingOnlyWhenANewMeetingWantsOne(t *testing.T) {
	start := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name         string
		video        bool
		eventID      string
		calendar     string
		wantProvider string
		wantBody     bool
	}{
		{"new with video", true, "", `{"defaultOnlineMeetingProvider":"teamsForBusiness"}`, "teamsForBusiness", true},
		{"new with video, default unset", true, "", `{"defaultOnlineMeetingProvider":"unknown","allowedOnlineMeetingProviders":["unknown","teamsForBusiness"]}`, "teamsForBusiness", true},
		{"new with video, calendar offers none", true, "", `{"defaultOnlineMeetingProvider":"unknown","allowedOnlineMeetingProviders":[]}`, "", true},
		{"new without video", false, "", "", "", true},
		{"reschedule keeps the meeting body", true, "immutable-event", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sent map[string]json.RawMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if tc.calendar == "" {
						t.Error("read the calendar's provider when no online meeting was wanted")
					}
					if _, err := w.Write([]byte(tc.calendar)); err != nil {
						t.Error(err)
					}
					return
				}
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Error(err)
				}
				reply := scheduledEvent{ID: "immutable-event"}
				if tc.wantProvider != "" {
					reply.OnlineMeeting = &onlineMeeting{JoinURL: teamsLink}
				}
				if err := json.NewEncoder(w).Encode(reply); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			api := &httpAPI{client: server.Client(), base: server.URL}
			in := connector.CalendarAppointment{CalendarID: "primary", EventID: tc.eventID, RequestID: "request", VideoCall: tc.video, Start: start, End: start.Add(30 * time.Minute)}
			receipt, err := api.Save(context.Background(), "token", in)
			if err != nil {
				t.Fatal(err)
			}
			var provider string
			if raw, ok := sent["onlineMeetingProvider"]; ok {
				if err := json.Unmarshal(raw, &provider); err != nil {
					t.Fatal(err)
				}
			}
			_, online := sent["isOnlineMeeting"]
			if provider != tc.wantProvider || online != (tc.wantProvider != "") {
				t.Fatalf("provider = %q online = %v, want %q", provider, online, tc.wantProvider)
			}
			if _, body := sent["body"]; body != tc.wantBody {
				t.Fatalf("body sent = %v, want %v", body, tc.wantBody)
			}
			if want := map[bool]string{true: teamsLink, false: ""}[tc.wantProvider != ""]; receipt.VideoURL != want {
				t.Fatalf("join link = %q, want %q", receipt.VideoURL, want)
			}
		})
	}
}

func TestARecoveredOutlookReceiptCarriesItsJoinLink(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		body := `{"id":"immutable-event","start":{"dateTime":"2026-10-05T10:00:00","timeZone":"UTC"},"end":{"dateTime":"2026-10-05T10:30:00","timeZone":"UTC"},"onlineMeeting":{"joinUrl":"` + teamsLink + `"}}`
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	start := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	receipt, err := api.Lookup(context.Background(), "token", connector.CalendarAppointment{CalendarID: "primary", EventID: "immutable-event", Start: start, End: start.Add(30 * time.Minute)})
	if err != nil || receipt == nil || receipt.VideoURL != teamsLink {
		t.Fatalf("receipt = %+v, err = %v", receipt, err)
	}
}
