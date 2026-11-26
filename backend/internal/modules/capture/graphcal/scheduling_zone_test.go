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

func savedEventTimes(t *testing.T, in connector.CalendarAppointment) (scheduledTime, scheduledTime) {
	t.Helper()
	var sent scheduledEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Error(err)
		}
		if err := json.NewEncoder(w).Encode(scheduledEvent{ID: "event"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	api := &httpAPI{client: server.Client(), base: server.URL}
	if _, err := api.Save(context.Background(), "token", in); err != nil {
		t.Fatal(err)
	}
	return sent.Start, sent.End
}

func TestOutlookEventKeepsTheChosenLocalClock(t *testing.T) {
	start := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, zone, eventID          string
		wantStart, wantEnd, wantZone string
	}{
		{"booked in Berlin", "Europe/Berlin", "", "2026-10-05T10:00:00", "2026-10-05T10:30:00", "W. Europe Standard Time"},
		{"rescheduled in Berlin", "Europe/Berlin", "event", "2026-10-05T10:00:00", "2026-10-05T10:30:00", "W. Europe Standard Time"},
		{"a zone Graph lists no IANA name for", "Europe/Vienna", "", "2026-10-05T10:00:00", "2026-10-05T10:30:00", "W. Europe Standard Time"},
		{"a zone known by its newer name", "Asia/Kolkata", "", "2026-10-05T13:30:00", "2026-10-05T14:00:00", "India Standard Time"},
		{"a zone a newer CLDR moved to a new Windows zone", "America/Vancouver", "", "2026-10-05T08:00:00", "2026-10-05T08:30:00", calendarUTC},
		{"another moved zone", "America/Winnipeg", "", "2026-10-05T08:00:00", "2026-10-05T08:30:00", calendarUTC},
		{"a moved zone known by an alias", "Canada/Pacific", "", "2026-10-05T08:00:00", "2026-10-05T08:30:00", calendarUTC},
		{"an unmoved zone beside a moved one", "America/Los_Angeles", "", "2026-10-05T01:00:00", "2026-10-05T01:30:00", "Pacific Standard Time"},
		{"an instant with no chosen zone", "", "", "2026-10-05T08:00:00", "2026-10-05T08:30:00", calendarUTC},
		{"a zone Windows has no name for", "Antarctica/Troll", "", "2026-10-05T08:00:00", "2026-10-05T08:30:00", calendarUTC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := connector.CalendarAppointment{
				CalendarID: "primary", EventID: tc.eventID, RequestID: "request", Start: start, End: start.Add(30 * time.Minute),
				StartWall: "2026-10-05T10:00:00", StartZone: tc.zone,
			}
			gotStart, gotEnd := savedEventTimes(t, in)
			want := [2]scheduledTime{{tc.wantStart, tc.wantZone}, {tc.wantEnd, tc.wantZone}}
			if [2]scheduledTime{gotStart, gotEnd} != want {
				t.Fatalf("sent start %+v end %+v, want %+v", gotStart, gotEnd, want)
			}
		})
	}
}

func TestUnreadableZoneDataIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, mapping, aliases string }{
		{"mapping is not XML", "<supplementalData", string(cldrZoneAliases)},
		{"aliases are not XML", string(cldrWindowsZones), "<ldmlBCP47"},
		{"mapping names no zone", "<supplementalData/>", string(cldrZoneAliases)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if zones, err := windowsZones([]byte(tc.mapping), []byte(tc.aliases), nil); err == nil {
				t.Fatalf("read %d zones from unreadable data", len(zones))
			}
		})
	}
}
