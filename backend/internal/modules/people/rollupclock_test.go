// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// One clock decides which FX rate an account's pipeline converts at.
//
// The rollup used to read the database's CURRENT_DATE while org360 sampled Go's,
// so two readers of one response could select rates a whole day apart — at
// midnight in the database's zone, and only then. A money figure that is wrong
// rarely and cannot be reproduced afterwards is the worst kind: the product's
// own defence against the report is indistinguishable from the bug.

import (
	"testing"
	"time"
)

func TestTheAsOfDateComesFromTheInjectedClock(t *testing.T) {
	original := rollupClock
	t.Cleanup(func() { rollupClock = original })

	frozen := time.Date(2026, 6, 2, 14, 30, 0, 0, time.UTC)
	rollupClock = func() time.Time { return frozen }

	if got := rollupAsOf(); !got.Equal(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("as-of = %s, want the 2nd — a date read from anywhere but the caller's clock is "+
			"the second clock this change exists to remove", got)
	}
}

// Two calls within one response answer the same date even across a midnight the
// wall clock crosses, because the caller binds it once. What this holds is the
// narrower half a unit test can: the date does not depend on the time of day.
func TestTheAsOfDateDoesNotMoveWithTheTimeOfDay(t *testing.T) {
	original := rollupClock
	t.Cleanup(func() { rollupClock = original })

	day := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	for _, hour := range []int{0, 9, 23} {
		rollupClock = func() time.Time { return day.Add(time.Duration(hour) * time.Hour) }
		if got := rollupAsOf(); !got.Equal(day) {
			t.Errorf("at %02d:00 the as-of date is %s, want %s — a rate selected by the hour is "+
				"one two readers of the same page can disagree about", hour, got, day)
		}
	}
}

// UTC, not the server's local zone. The rate table is keyed by date, and a
// local-zone answer makes one installation convert differently depending on
// where its process happens to run.
func TestTheAsOfDateIsUTC(t *testing.T) {
	original := rollupClock
	t.Cleanup(func() { rollupClock = original })

	// 01:00 on the 3rd in a +02:00 zone is still the 2nd in UTC.
	east := time.FixedZone("east", 2*60*60)
	rollupClock = func() time.Time { return time.Date(2026, 6, 3, 1, 0, 0, 0, east) }

	if got := rollupAsOf(); !got.Equal(time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("as-of = %s, want the 2nd in UTC — reading the server's zone makes the same "+
			"installation convert differently depending on where it runs", got)
	}
}
