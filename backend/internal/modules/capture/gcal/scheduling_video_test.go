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

const meetLink = "https://meet.google.com/abc-defg-hij"

// videoCalendar records what Save sent and answers the insert with created,
// and any later read of the event with read.
type videoCalendar struct {
	readFails bool
	sent      scheduledEvent
	query     string
	reads     int
	created   scheduledEvent
	read      scheduledEvent
}

func (c *videoCalendar) serve(t *testing.T) *httpAPI {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := c.read
		if r.Method == http.MethodGet {
			c.reads++
			if c.readFails {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
		} else {
			c.query = r.URL.RawQuery
			if err := json.NewDecoder(r.Body).Decode(&c.sent); err != nil {
				t.Error(err)
			}
			reply = c.created
		}
		if err := json.NewEncoder(w).Encode(reply); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return &httpAPI{client: server.Client(), base: server.URL}
}

func videoAppointment(video bool, eventID string) connector.CalendarAppointment {
	return connector.CalendarAppointment{
		CalendarID: "primary", EventID: eventID, RequestID: "01234567-89ab-cdef-0123-456789abcdef", VideoCall: video,
		Start: time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC), End: time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC),
	}
}

func conferenceIn(status string) *conferenceData {
	return &conferenceData{CreateRequest: &conferenceRequest{Status: &conferenceStatus{Code: status}}}
}

func TestANewMeetingAsksGoogleForAMeetKeyedToItsRequest(t *testing.T) {
	calendar := &videoCalendar{created: scheduledEvent{ID: "event", VideoURL: meetLink, Conference: conferenceIn("success")}}
	receipt, err := calendar.serve(t).Save(context.Background(), "token", videoAppointment(true, ""))
	if err != nil {
		t.Fatal(err)
	}
	request := calendar.sent.Conference
	if request == nil || request.CreateRequest == nil || request.CreateRequest.RequestID != "01234567-89ab-cdef-0123-456789abcdef" ||
		request.CreateRequest.SolutionKey == nil || request.CreateRequest.SolutionKey.Type != "hangoutsMeet" {
		t.Fatalf("conference request = %+v", request)
	}
	if calendar.query != "sendUpdates=all&conferenceDataVersion=1" {
		t.Fatalf("query = %q, want conference data enabled", calendar.query)
	}
	if receipt.VideoURL != meetLink || calendar.reads != 0 {
		t.Fatalf("receipt = %+v after %d reads", receipt, calendar.reads)
	}
}

func TestGoogleIsAskedForAMeetOnlyWhenANewMeetingWantsOne(t *testing.T) {
	for name, in := range map[string]connector.CalendarAppointment{
		"video off":  videoAppointment(false, ""),
		"reschedule": videoAppointment(true, "event"),
	} {
		t.Run(name, func(t *testing.T) {
			calendar := &videoCalendar{created: scheduledEvent{ID: "event", VideoURL: meetLink}}
			receipt, err := calendar.serve(t).Save(context.Background(), "token", in)
			if err != nil {
				t.Fatal(err)
			}
			if calendar.sent.Conference != nil || calendar.query != "sendUpdates=all" {
				t.Fatalf("sent conference %+v with query %q", calendar.sent.Conference, calendar.query)
			}
			if receipt.VideoURL != meetLink {
				t.Fatalf("an existing Meet link was not read back: %+v", receipt)
			}
		})
	}
}

func TestAPendingMeetIsReadOnceAndNeverFailsTheDelivery(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    string
		later     string
		readFails bool
		wantReads int
		wantLink  string
	}{
		{"pending then ready", "pending", meetLink, false, 1, meetLink},
		{"still pending", "pending", "", false, 1, ""},
		{"read unavailable", "pending", meetLink, true, 1, ""},
		{"refused", "failure", meetLink, false, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calendar := &videoCalendar{
				readFails: tc.readFails,
				created:   scheduledEvent{ID: "event", Conference: conferenceIn(tc.status)},
				read:      scheduledEvent{ID: "event", VideoURL: tc.later},
			}
			receipt, err := calendar.serve(t).Save(context.Background(), "token", videoAppointment(true, ""))
			if err != nil {
				t.Fatalf("a missing video link failed a delivered event: %v", err)
			}
			if receipt.EventID != "event" || receipt.VideoURL != tc.wantLink || calendar.reads != tc.wantReads {
				t.Fatalf("receipt = %+v after %d reads, want link %q after %d", receipt, calendar.reads, tc.wantLink, tc.wantReads)
			}
		})
	}
}

func TestARecoveredGoogleReceiptCarriesItsMeetLink(t *testing.T) {
	in := videoAppointment(true, "")
	calendar := &videoCalendar{read: scheduledEvent{ID: "0123456789abcdef0123456789abcdef", Status: "confirmed", VideoURL: meetLink, Start: eventTime{in.Start}, End: eventTime{in.End}}}
	receipt, err := calendar.serve(t).Lookup(context.Background(), "token", in)
	if err != nil || receipt == nil || receipt.VideoURL != meetLink {
		t.Fatalf("receipt = %+v, err = %v", receipt, err)
	}
}
