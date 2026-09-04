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

	got := dismissalDeadline(nanosecondClock, 3)

	if remainder := got.Nanosecond() % int(time.Microsecond/time.Nanosecond); remainder != 0 {
		t.Errorf("the deadline carries %dns below a microsecond (%s) — timestamptz stores "+
			"microseconds, so the audit would claim a precision the row does not have and "+
			"a re-dismissal's before-image would not match it", remainder, got.Format(time.RFC3339Nano))
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
