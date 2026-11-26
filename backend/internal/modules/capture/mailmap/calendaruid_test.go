// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func calendarUIDOf(t *testing.T, raw []byte) string {
	t.Helper()
	msg, err := Parse(raw, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("gmail", nil).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("an email record carries ActivityFields")
	}
	return fields.CalendarUID
}

func TestAnInvitationNamesTheEventItIsAbout(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		part []string
		want string
	}{{
		name: "the event's UID, folded across two lines",
		part: []string{
			"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
			"",
			"BEGIN:VCALENDAR", "METHOD:REQUEST",
			"BEGIN:VTIMEZONE", "UID:not-the-event", "END:VTIMEZONE",
			"BEGIN:VEVENT", "UID:7kq2abc", " def@google.com", "END:VEVENT",
			"END:VCALENDAR",
		},
		want: "7kq2abcdef@google.com",
	}, {
		name: "no event in the calendar",
		part: []string{
			"Content-Type: text/calendar; method=REQUEST",
			"",
			"BEGIN:VCALENDAR", "METHOD:REQUEST", "END:VCALENDAR",
		},
		want: "",
	}, {
		name: "a part whose two methods disagree names no event",
		part: []string{
			"Content-Type: text/calendar; method=REPLY",
			"",
			"BEGIN:VCALENDAR", "METHOD:REQUEST",
			"BEGIN:VEVENT", "UID:disputed@example.com", "END:VEVENT",
			"END:VCALENDAR",
		},
		want: "",
	}, {
		name: "a UID longer than a row stores",
		part: []string{
			"Content-Type: text/calendar; method=REQUEST",
			"",
			"BEGIN:VCALENDAR", "METHOD:REQUEST",
			"BEGIN:VEVENT", "UID:" + strings.Repeat("x", maxCalendarUIDLength+1), "END:VEVENT",
			"END:VCALENDAR",
		},
		want: "",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := calendarUIDOf(t, declineFrom(tc.part...)); got != tc.want {
				t.Errorf("CalendarUID = %q, want %q", got, tc.want)
			}
		})
	}
}
