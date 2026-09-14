// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The two properties that keep a notice case honest about a deadline, held
// where they can be held without a database.

import (
	"testing"
	"time"
)

// TestOverdueIsNotAState is the design decision, written down where breaking it
// fails.
//
// Overdue is a reading of the clock against due_at. Were it a stored state, a
// case would become overdue only once a sweep had written that value — so a job
// that stopped firing would leave every late case reading as on time, with
// nothing anywhere failing to say so. That is the one way this control must not
// break, because under-reporting a duty looks exactly like having no duties.
//
// data_subject_request settles it the same way: no overdue in its status
// vocabulary, and OpenDSRsDueSoonest (dsrlane.go) orders by due_at. If somebody
// adds an 'overdue' state here, this test says why not to.
func TestOverdueIsNotAState(t *testing.T) {
	for _, s := range unresolvedNoticeStates() {
		if s == "overdue" {
			t.Fatal("'overdue' became a notice state: a stored overdue is written by a sweep, " +
				"so a sweep that stops running makes every late case read as on time. Order by " +
				"due_at instead — the clock is the authority, not a column")
		}
	}
}

// TestTheQueueAsksForEveryUnresolvedState keeps the lane's idea of "still owed"
// tied to the states themselves.
//
// A state added to the vocabulary and forgotten here would be invisible to the
// queue: cases would sit in it, owed and unshown. The count is asserted rather
// than the membership, so adding a state fails this until somebody decides
// whether the queue should show it.
func TestTheQueueAsksForEveryUnresolvedState(t *testing.T) {
	got := unresolvedNoticeStates()
	// `assigned` is here because somebody having taken a duty is not the same
	// as having discharged it. A case that left the queue on being claimed
	// would be owed, worked and invisible — and the only seat that would still
	// see it is the one that claimed it, which is exactly the wrong place to
	// put the reminder.
	//
	// The two excusing states are NOT here, and that is the decision this test
	// forced. `provided_elsewhere` and `exempt_with_reason` both END the duty:
	// one says it was met, the other says it never applied. Keeping either on
	// the queue would prompt work nobody owes, and an officer who had already
	// written down why would be asked again tomorrow.
	// `delivery_failed` is here because a disclosure that did not arrive left
	// the duty exactly as owed as before it was sent. The subject was not told.
	// A case resting outside the queue there would be the worst of the eight
	// states to get wrong: it reads as handled, so nobody looks at it again,
	// and the duty quietly stops being anybody's — which is the failure the
	// whole table exists to prevent.
	want := []string{"open", "assigned", "queued", "delivery_failed", "blocked"}
	if len(got) != len(want) {
		t.Fatalf("the queue asks for %v, want %v — a state was added to the vocabulary without "+
			"deciding whether a case sitting in it is still owed", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("position %d is %q, want %q", i, got[i], w)
		}
	}
}

// TestABlockedCaseIsStillOwed — blocked means "we cannot discharge this yet",
// not "we no longer owe it". A blocked case that dropped off the queue would be
// a duty nobody is ever reminded of again, which is how an obligation quietly
// stops being anybody's.
func TestABlockedCaseIsStillOwed(t *testing.T) {
	var found bool
	for _, s := range unresolvedNoticeStates() {
		if s == string(NoticeBlocked) {
			found = true
		}
	}
	if !found {
		t.Error("a blocked notice case left the queue: blocked says we cannot discharge it yet, " +
			"not that we stopped owing it, and a duty off every list is one nobody discharges")
	}
}

// TestDataFromTheSubjectOwesNoCase pins the half of the split that writes
// nothing, and why it writes nothing rather than not_required.
//
// A not_required row would assert the disclosure was made at collection. That
// is an assumption, not evidence — and the rule for not_required is that it
// belongs only where the disclosure IS evidenced. Absence of a case is the
// honest record: the acquisition row already says how the contact arrived.
func TestDataFromTheSubjectOwesNoCase(t *testing.T) {
	for _, kind := range []string{
		"subject_initiated", "customer_contract",
		"requested_quote_or_meeting", "in_person_permission",
	} {
		duty, owed := DutyFor(kind)
		if owed {
			t.Errorf("%s opens a %s case: the subject handed us the data, so Art. 13 was "+
				"discharged by the surface that took it", kind, duty.Rule)
		}
	}
}

// TestAnUnknownKindTakesTheStrictReading — the census above fails when the
// vocabulary grows, but only if somebody runs it. Until then the default must
// not go quiet: a contact whose origin nobody knows is exactly the one most
// likely to be owed a notice.
func TestAnUnknownKindTakesTheStrictReading(t *testing.T) {
	duty, owed := DutyFor("harvested_from_somewhere_new")
	if !owed || duty.Rule != RuleArt14 {
		t.Errorf("an unrecognised acquisition kind owes %v/%q, want an Art. 14 case: a duty "+
			"wrongly raised costs somebody a look, one wrongly skipped is invisible", owed, duty.Rule)
	}
}

// TestAMonthClampsToTheEndOfTheTargetMonth pins the arithmetic a deadline rests
// on, at the dates where the two obvious implementations are both wrong.
//
// time.AddDate normalizes forward, so 31 January plus one month is 3 March —
// three days past the deadline, in the direction that favours the controller.
// A fixed 30*24h is wrong the other way and loses a day across a daylight-saving
// boundary. Neither is "one month", so this holds the third answer.
func TestAMonthClampsToTheEndOfTheTargetMonth(t *testing.T) {
	utc := time.UTC
	for _, tc := range []struct {
		name string
		from time.Time
		want string
	}{
		{
			"the 31st into February clamps to the 28th",
			time.Date(2026, time.January, 31, 9, 0, 0, 0, utc), "2026-02-28",
		},
		{
			"the 31st into a leap February clamps to the 29th",
			time.Date(2028, time.January, 31, 9, 0, 0, 0, utc), "2028-02-29",
		},
		{
			"the 31st into a 30-day month clamps to the 30th",
			time.Date(2026, time.March, 31, 9, 0, 0, 0, utc), "2026-04-30",
		},
		{
			"an ordinary date keeps its day",
			time.Date(2026, time.March, 14, 9, 0, 0, 0, utc), "2026-04-14",
		},
		{
			"the end of the year rolls over",
			time.Date(2026, time.December, 15, 9, 0, 0, 0, utc), "2027-01-15",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := AddMonths(tc.from, 1).Format("2006-01-02")
			if got != tc.want {
				t.Errorf("%s + 1 month = %s, want %s — a deadline past the end of the target "+
					"month gives the controller time the regulation does not",
					tc.from.Format("2006-01-02"), got, tc.want)
			}
		})
	}
	// Zero months is the Art. 13 case: owed at collection, not a month later.
	at := time.Date(2026, time.January, 31, 9, 0, 0, 0, utc)
	if got := AddMonths(at, 0); !got.Equal(at) {
		t.Errorf("zero months moved the clock to %v, want it untouched", got)
	}
}
