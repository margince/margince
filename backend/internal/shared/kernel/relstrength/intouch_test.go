// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
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
			got := ReadInTouch(instantAt(optionalDaysAgo(tc.inbound)), instantAt(optionalDaysAgo(tc.met)), instantAt(optionalDaysAhead(tc.booked)), now, time.UTC)
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

// A message dated ahead of now, scheduled or mis-stamped, has not reached
// anybody yet and must not keep a quiet account off "at risk".
func TestAMessageDatedAheadIsNotContact(t *testing.T) {
	t.Parallel()
	got := ReadInTouch(instantAt(optionalDaysAhead(3)), nil, nil, now, time.UTC)
	if got.Basis != InTouchNever || got.Current() {
		t.Fatalf("reading = %s (current %v), want never — the message has not been sent yet", got.Basis, got.Current())
	}
}

// A meeting passed as the last one but dated ahead has not been held, and
// counts only as the booking the caller passes separately.
func TestAMeetingAheadIsNotOneHeld(t *testing.T) {
	t.Parallel()
	got := ReadInTouch(instantAt(daysAgo(77)), instantAt(optionalDaysAhead(3)), nil, now, time.UTC)
	if got.Basis != InTouchQuiet {
		t.Fatalf("basis = %s, want %s — a meeting that has not happened yet is not contact we had", got.Basis, InTouchQuiet)
	}
}

// A message logged for today's date has been written from the start of that
// date, before its stored noon, and is zero days old. Auckland's morning is
// still the evening before by UTC.
func TestAMessageDatedTodayIsContactBeforeItsStoredNoon(t *testing.T) {
	t.Parallel()
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	morning := time.Date(2026, 3, 10, 9, 0, 0, 0, auckland)
	today := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	got := ReadInTouch(&occurredday.Moment{At: today, DateOnly: true}, nil, nil, morning, auckland)
	if got.Basis != InTouchWrote || got.Days != 0 {
		t.Fatalf("reading = %s after %d days, want %s after 0", got.Basis, got.Days, InTouchWrote)
	}
}

// A day meeting dated today is still ahead until its date ends, so it is booked
// rather than met. Once the date is over it is a meeting we had.
func TestADayMeetingDatedTodayIsBookedUntilItsDateEnds(t *testing.T) {
	t.Parallel()
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	meeting := &occurredday.Moment{At: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC), DateOnly: true}
	evening := time.Date(2026, 3, 10, 23, 0, 0, 0, auckland)
	if got := ReadInTouch(nil, meeting, meeting, evening, auckland); got.Basis != InTouchBooked || got.BookedAt != meeting {
		t.Errorf("on its date the reading is %s, want %s by that meeting", got.Basis, InTouchBooked)
	}
	nextMorning := time.Date(2026, 3, 11, 0, 30, 0, 0, auckland)
	if got := ReadInTouch(nil, meeting, meeting, nextMorning, auckland); got.Basis != InTouchMet || got.BookedAt != nil {
		t.Errorf("after its date the reading is %s booked %v, want %s and nothing booked", got.Basis, got.BookedAt, InTouchMet)
	}
}

// The interaction rule counts a meeting only once someone recorded it held. A
// meeting with no status, booked, canceled or a no-show is no exchange.
func TestTheCountingRuleCountsOnlyAHeldMeeting(t *testing.T) {
	t.Parallel()
	rule := InteractionCountsSQL("a")
	if !strings.Contains(rule, "a.kind IN "+InteractionKindSQLGroup()) {
		t.Fatalf("rule %q does not filter on the shared interaction kinds", rule)
	}
	if !strings.Contains(rule, HadContactSQL("a")) || !strings.Contains(HadContactSQL("a"), meetingoutcome.HeldSQL("a")) {
		t.Fatalf("rule %q does not hold a meeting to a recorded held outcome", rule)
	}
	for _, clock := range []string{rule, LastActivityCountsSQL("a", "now()"), DirectionClockSQL("a")} {
		if strings.Contains(clock, "IS NULL OR a.meeting_status") || strings.Contains(clock, "'booked'") {
			t.Errorf("contact rule %q admits a meeting nobody reported held", clock)
		}
	}
}

// The Go predicate answers every meeting status as the SQL one does: held is
// contact, every other status and none is not.
func TestTheGoAndSQLMeetingRulesAgree(t *testing.T) {
	t.Parallel()
	for status, want := range map[string]bool{"": false, "booked": false, "held": true, "canceled": false, "no_show": false} {
		if got := InteractionCounts("meeting", status, "", ""); got != want {
			t.Errorf("a meeting with status %q counts: %v, want %v", status, got, want)
		}
		if got := HadContact("meeting", status, ""); got != want {
			t.Errorf("HadContact for a meeting with status %q = %v, want %v", status, got, want)
		}
	}
	if !InteractionCounts("email", "", "", "") || InteractionCounts("note", "", "", "") {
		t.Error("InteractionCounts must admit an email and refuse a note")
	}
}

// A meeting nobody called off is a broader test than contact: booked, held and
// untracked meetings pass it, canceled and no-show ones do not.
func TestANotCalledOffMeetingIsAnyButCanceledOrNoShow(t *testing.T) {
	t.Parallel()
	meeting := MeetingNotCalledOffSQL("a")
	for _, status := range []string{"booked", "held", "canceled", "no_show"} {
		inSQL := strings.Contains(meeting, "'"+status+"'")
		if MeetingStatusNotCalledOff(status) != inSQL {
			t.Errorf("status %q: Go admits it = %v, SQL %q admits it = %v", status, !inSQL, meeting, inSQL)
		}
	}
	if !MeetingStatusNotCalledOff("") || !strings.Contains(meeting, "IS NULL") {
		t.Error("an untracked meeting is not called off in both rules")
	}
}

// Only a call that reached somebody is contact: Reached and Put through, and
// a call logged with no outcome, as every call was before the outcome existed.
// The SQL and the Go predicate render from one list.
func TestOnlyACallThatReachedSomebodyCounts(t *testing.T) {
	t.Parallel()
	for outcome, want := range map[string]bool{
		"": true, "reached": true, "put_through": true,
		"not_reached": false, "voicemail": false, "call_back_later": false,
	} {
		if got := InteractionCounts("call", "", outcome, ""); got != want {
			t.Errorf("a call with outcome %q counts: %v, want %v", outcome, got, want)
		}
		if got := strings.Contains(CallCountsSQL("a"), "'"+outcome+"'"); outcome != "" && got != want {
			t.Errorf("the SQL rule lists %q: %v, want %v", outcome, got, want)
		}
	}
	if rule := HadContactSQL("a"); !strings.Contains(rule, CallCountsSQL("a")) || !strings.Contains(rule, meetingoutcome.HeldSQL("a")) {
		t.Errorf("HadContactSQL %q must carry both the meeting and the call rule", rule)
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

// Held and ahead split a meeting by its outcome and the clock. Only a meeting
// recorded held that already took place is one we met them at; a past booking
// is awaiting its outcome. Only an untracked or booked meeting still to come
// counts as one ahead.
func TestHeldAndAheadSplitByOutcomeAndClock(t *testing.T) {
	t.Parallel()
	past := MeetingHeldByNowSQL("m", "$9")
	if !strings.Contains(past, meetingoutcome.HeldSQL("m")) || !strings.Contains(past, occurredday.MeetingTookPlaceSQL("m", "$9")) {
		t.Errorf("held by now = %q, want a meeting recorded held that already took place", past)
	}
	if strings.Contains(past, "IS NULL") || strings.Contains(past, "booked") {
		t.Errorf("held by now = %q admits a meeting nobody reported held", past)
	}
	ahead := MeetingAheadSQL("m", "$9")
	if !strings.Contains(ahead, occurredday.AheadSQL("m", "$9")) || strings.Contains(ahead, "held") {
		t.Errorf("ahead = %q, want an untracked or booked meeting still to come", ahead)
	}
}

// The draft's history admits a meeting only when it was held, and a note
// unless it was called off.
func TestTheDraftHistoryAdmitsOnlyAHeldMeeting(t *testing.T) {
	t.Parallel()
	rule := HappenedByNowSQL("a", "$1")
	if want := "(a.kind <> 'meeting' OR " + meetingoutcome.HeldSQL("a") + ")"; HeldWhenMeetingSQL("a") != want || !strings.Contains(rule, want) {
		t.Errorf("history rule %q does not hold a meeting to a recorded held outcome", rule)
	}
	if !strings.Contains(rule, "a.kind <> 'note' OR "+MeetingNotCalledOffSQL("a")) {
		t.Errorf("history rule %q no longer refuses a called-off note", rule)
	}
}

func TestOnlyContactKindsMoveTheLastActivityClock(t *testing.T) {
	t.Parallel()
	rule := LastActivityCountsSQL("a", "now()")
	const want = "a.kind IN ('email','message','meeting','call','note')"
	if !strings.Contains(rule, want) {
		t.Fatalf("LastActivityCountsSQL = %q, want it to carry %q", rule, want)
	}
	if strings.Contains(rule, "'task'") {
		t.Fatalf("LastActivityCountsSQL = %q names a task, so filing one moves the clock", rule)
	}
	if got := LastActivityKinds(); strings.Join(got, ",") != "email,message,meeting,call,note" {
		t.Fatalf("LastActivityKinds() = %v", got)
	}
}
