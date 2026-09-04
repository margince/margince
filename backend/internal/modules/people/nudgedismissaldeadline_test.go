// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The deadline a dismissal records is one the row can hold.
//
// A unit test rather than an integration one, deliberately: the defect only
// APPEARS when the host clock has sub-microsecond precision, so an integration
// test asserting the trail joins up passes on a macOS developer's machine
// whatever the code does and fails on a Linux runner. Held here it is the same
// answer on both.

import (
	"testing"
	"time"
)

// A clock with a sub-microsecond remainder, which is what a Linux `time.Now()`
// hands over and a macOS one usually does not.
var nanosecondClock = time.Date(2026, 9, 4, 6, 46, 20, 317_417_328, time.UTC)

func TestADismissalDeadlineCarriesNoPrecisionTheColumnCannotHold(t *testing.T) {
	t.Parallel()

	// The exact instant, not merely one aligned to a microsecond. Alignment
	// alone is satisfied by truncating to the millisecond or the second, which
	// would move the expiry while still reading as a precision fix — so the
	// assertion names the deadline that must come out: the clock plus three
	// days, with the 328ns remainder gone and NOTHING else removed.
	want := time.Date(2026, 9, 7, 6, 46, 20, 317_417_000, time.UTC)

	if got := dismissalDeadline(nanosecondClock, 3); !got.Equal(want) {
		t.Errorf("the deadline is %s, want %s — timestamptz stores microseconds, so the "+
			"audit must claim that precision and no other: less and the dismissal lapses "+
			"at a different moment, more and a re-dismissal's before-image will not match it",
			got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

// The truncation must not move the deadline to another day or another second:
// it is a precision fix, and a dismissal that lapsed a second early would be a
// behaviour change wearing its clothes.
func TestTruncatingTheDeadlineKeepsTheDayAndTheSecond(t *testing.T) {
	t.Parallel()

	for _, days := range []int{0, 1, 3, 30, 90} {
		want := nanosecondClock.UTC().AddDate(0, 0, days)
		got := dismissalDeadline(nanosecondClock, days)
		if !got.Truncate(time.Second).Equal(want.Truncate(time.Second)) {
			t.Errorf("a %d-day dismissal lands at %s, want the same second as %s",
				days, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
		}
	}
}
