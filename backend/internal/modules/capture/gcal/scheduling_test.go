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
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok || unavailable.Calendar != "private-calendar" || unavailable.Reason != "omitted" {
		t.Fatalf("missing selected calendar was not reported as unavailable: %v", err)
	}
}

// Google's reason separates a deleted calendar from a revoked grant, so it must
// reach the error. A reason that is prose must not. Only notFound goes on to the
// events read, which here cannot find the calendar either.
func TestAGoogleCalendarErrorCarriesItsReasonToTheError(t *testing.T) {
	for reason, want := range map[string]string{
		"notFound":                      "notFound",
		"forbidden":                     "forbidden",
		"Not a code\nlevel=ERROR forge": "",
	} {
		api, eventReads := splitCalendarServer(t, []string{reason}, func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusNotFound)
		})
		_, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 0, 1))
		unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
		if !ok || unavailable.Reason != want || strings.ContainsAny(err.Error(), "\n") {
			t.Errorf("reason %q → %v, want reason %q on one line", reason, err, want)
		}
		if reads := *eventReads; (reason == freeBusyNotFound) != (reads > 0) {
			t.Errorf("reason %q read the events %d times", reason, reads)
		}
	}
}

const testCalendar = "en.usa#holiday@group.v.calendar.google.com"

var testFrom = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// splitCalendarServer answers freeBusy with the given errors for testCalendar
// and the events read with events. It counts the events reads.
func splitCalendarServer(t *testing.T, freeBusyErrors []string, events func(w http.ResponseWriter)) (*httpAPI, *int) {
	t.Helper()
	reads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/freeBusy") {
			found := busyCalendar{}
			for _, reason := range freeBusyErrors {
				found.Errors = append(found.Errors, struct {
					Reason string `json:"reason"`
				}{reason})
			}
			if err := json.NewEncoder(w).Encode(map[string]map[string]busyCalendar{"calendars": {testCalendar: found}}); err != nil {
				t.Error(err)
			}
			return
		}
		reads++
		events(w)
	}))
	t.Cleanup(server.Close)
	return &httpAPI{client: server.Client(), base: server.URL}, &reads
}

func eventsAnswer(t *testing.T, body string) func(w http.ResponseWriter) {
	t.Helper()
	return func(w http.ResponseWriter) {
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}
}

// Google's freeBusy answers notFound for a public holiday calendar the events
// read serves. Holidays marked free block nothing.
func TestAHolidayCalendarIsReadLikeAnyOtherCalendar(t *testing.T) {
	api, reads := splitCalendarServer(t, []string{freeBusyNotFound}, eventsAnswer(t,
		`{"timeZone":"UTC","items":[{"id":"h","status":"confirmed","transparency":"transparent","start":{"date":"2026-01-05"},"end":{"date":"2026-01-06"}}]}`))
	busy, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 1, 0))
	if err != nil || len(busy) != 0 || *reads != 1 {
		t.Fatalf("busy=%v err=%v event reads=%d, want nothing busy after one events read", busy, err, *reads)
	}
}

// A holiday the host marked busy blocks its whole local day.
func TestABusyHolidayStillBlocksItsDay(t *testing.T) {
	api, _ := splitCalendarServer(t, []string{freeBusyNotFound}, eventsAnswer(t,
		`{"timeZone":"Europe/Berlin","items":[{"id":"h","status":"confirmed","start":{"date":"2026-01-05"},"end":{"date":"2026-01-06"}}]}`))
	busy, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 1, 0))
	berlin, zoneErr := time.LoadLocation("Europe/Berlin")
	if zoneErr != nil {
		t.Fatal(zoneErr)
	}
	start := time.Date(2026, 1, 5, 0, 0, 0, 0, berlin)
	if err != nil || len(busy) != 1 || !busy[0].Start.Equal(start) || !busy[0].End.Equal(start.AddDate(0, 0, 1)) {
		t.Fatalf("busy=%v err=%v, want the local day of 5 January", busy, err)
	}
}

func TestACalendarNeitherEndpointCanFindIsUnavailable(t *testing.T) {
	api, _ := splitCalendarServer(t, []string{freeBusyNotFound}, func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) })
	_, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 1, 0))
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok || unavailable.Calendar != testCalendar || unavailable.Reason != freeBusyNotFound {
		t.Fatalf("err=%v, want the calendar unavailable as notFound", err)
	}
}

// freeBusy accepted the token, so the events read's 403 refuses this calendar
// alone. Read as a refused credential, it would ask the host to reconnect.
func TestACalendarTheEventsReadRefusesIsUnavailableNotDisconnected(t *testing.T) {
	api, _ := splitCalendarServer(t, []string{freeBusyNotFound}, func(w http.ResponseWriter) { w.WriteHeader(http.StatusForbidden) })
	_, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 1, 0))
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok || unavailable.Reason != "forbidden" || errors.Is(err, connector.ErrAuthRejected) {
		t.Fatalf("err=%v, want this calendar forbidden and the credential untouched", err)
	}
}

func TestARefusedCredentialOnTheEventsReadStaysACredentialFailure(t *testing.T) {
	api, _ := splitCalendarServer(t, []string{freeBusyNotFound}, func(w http.ResponseWriter) { w.WriteHeader(http.StatusUnauthorized) })
	_, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 1, 0))
	if !errors.Is(err, connector.ErrAuthRejected) {
		t.Fatalf("err=%v, want a refused credential", err)
	}
}

func TestFreeBusyNotFoundMixedWithAnotherReasonRefuses(t *testing.T) {
	api, reads := splitCalendarServer(t, []string{freeBusyNotFound, "backendError"}, eventsAnswer(t, `{"timeZone":"UTC","items":[]}`))
	_, err := api.Busy(context.Background(), "token", testCalendar, testFrom, testFrom.AddDate(0, 1, 0))
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok || unavailable.Reason != "backendError" || *reads != 0 {
		t.Fatalf("err=%v event reads=%d, want backendError and no events read", err, *reads)
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
