// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"strings"
	"testing"
	"time"
)

func TestReadInTouchCountsMeetingsAsContact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                     string
		inbound, met, booked     int // days ago; -1 is none, and booked counts days AHEAD
		wantBasis                string
		wantDays                 int
		wantCurrent, wantBooking bool
	}{
		{"they wrote recently", 3, -1, -1, InTouchWrote, 3, true, false},
		{"a meeting is newer than their last word", 77, 21, -1, InTouchMet, 21, true, false},
		{"their word is newer than the meeting", 2, 21, -1, InTouchWrote, 2, true, false},
		{"only meetings, never a message", -1, 5, -1, InTouchMet, 5, true, false},
		{"quiet, but a meeting is booked", 77, -1, 2, InTouchBooked, 77, true, true},
		{"never in touch, but a meeting is booked", -1, -1, 2, InTouchBooked, 0, true, true},
		{"quiet past the threshold on both", 77, 40, -1, InTouchQuiet, 40, false, false},
		{"on the threshold is still in touch", InTouchDays, -1, -1, InTouchWrote, InTouchDays, true, false},
		{"nothing, ever", -1, -1, -1, InTouchNever, 0, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ReadInTouch(optionalDaysAgo(tc.inbound), optionalDaysAgo(tc.met), optionalDaysAhead(tc.booked), now)
			if got.Basis != tc.wantBasis || got.Days != tc.wantDays {
				t.Fatalf("reading = %s after %d days, want %s after %d", got.Basis, got.Days, tc.wantBasis, tc.wantDays)
			}
			if got.Current() != tc.wantCurrent {
				t.Fatalf("current = %v, want %v", got.Current(), tc.wantCurrent)
			}
			if (got.BookedAt != nil) != tc.wantBooking {
				t.Fatalf("booked at = %v, want a booking: %v", got.BookedAt, tc.wantBooking)
			}
		})
	}
}

// A meeting passed as the last one but dated ahead has not been held, and
// counts only as the booking the caller passes separately.
func TestAMeetingAheadIsNotOneHeld(t *testing.T) {
	t.Parallel()
	got := ReadInTouch(daysAgo(77), optionalDaysAhead(3), nil, now)
	if got.Basis != InTouchQuiet {
		t.Fatalf("basis = %s, want %s — a meeting that has not happened yet is not contact we had", got.Basis, InTouchQuiet)
	}
}

// The SQL rule and the meeting statuses: canceled and no-show meetings are
// not contact, held, booked and untracked ones are.
func TestTheCountingRuleRefusesMeetingsThatDidNotHappen(t *testing.T) {
	t.Parallel()
	rule := InteractionCountsSQL("a")
	if !strings.Contains(rule, "a.kind IN "+InteractionKindSQLGroup()) {
		t.Fatalf("rule %q does not filter on the shared interaction kinds", rule)
	}
	meeting := MeetingCountsSQL("a")
	if !strings.Contains(rule, meeting) {
		t.Fatalf("rule %q does not apply the meeting status test %q", rule, meeting)
	}
	for _, status := range []string{"'booked'", "'held'", "IS NULL"} {
		if !strings.Contains(meeting, status) {
			t.Errorf("the meeting test %q does not admit %s", meeting, status)
		}
	}
	for _, status := range []string{"canceled", "no_show"} {
		if strings.Contains(meeting, status) {
			t.Errorf("the meeting test %q names %s, which is not contact", meeting, status)
		}
	}
}

func optionalDaysAgo(n int) *time.Time {
	if n < 0 {
		return nil
	}
	return daysAgo(n)
}

func optionalDaysAhead(n int) *time.Time {
	if n < 0 {
		return nil
	}
	return daysAgo(-n)
}

// Took place and ahead split a meeting by the clock: a booking that started
// counts as contact we had whether or not anybody marked it held, and only an
// untracked or booked meeting still to come counts as one ahead.
func TestTookPlaceAndAheadSplitByTheClock(t *testing.T) {
	t.Parallel()
	past := MeetingTookPlaceSQL("m", "$9")
	if !strings.Contains(past, MeetingCountsSQL("m")) || !strings.Contains(past, "m.occurred_at <= $9") {
		t.Errorf("took place = %q, want a meeting nobody called off that already started", past)
	}
	ahead := MeetingAheadSQL("m", "$9")
	if !strings.Contains(ahead, "m.occurred_at > $9") || strings.Contains(ahead, "held") {
		t.Errorf("ahead = %q, want an untracked or booked meeting still to come", ahead)
	}
}
