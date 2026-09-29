// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package clocktest

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/clockskew"
)

func TestAnOrdinaryRunDatesAFixtureFromWallTime(t *testing.T) {
	t.Setenv(clockskew.EnvVar, "")

	before := time.Now()
	got := Now(t)
	if got.Before(before) || got.After(time.Now()) {
		t.Fatalf("Now() = %v; want wall time, between %v and now", got, before)
	}
}

func TestTheFixtureApplierMovesTheBaseInstant(t *testing.T) {
	t.Setenv(clockskew.EnvVar, "fixture:200")

	shifted := Now(t).Sub(time.Now())
	// A tolerance, because the two clock reads are not the same instant — and
	// stated as a window around the offset rather than as an elapsed budget, so
	// the assertion does not become the wall-clock comparison mergegateclockbounds
	// prohibits.
	want := 200 * 24 * time.Hour
	if shifted < want-time.Minute || shifted > want+time.Minute {
		t.Fatalf("Now() is %v from wall time; want %v", shifted, want)
	}
}

// The other two appliers move a clock this helper must not move again.
func TestAnAppliedClockIsNotShiftedTwice(t *testing.T) {
	for _, value := range []string{"machine:200", "database:200"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(clockskew.EnvVar, value)

			if shifted := Now(t).Sub(time.Now()); shifted > time.Minute {
				t.Fatalf("Now() is %v from wall time under %q; want wall time — the clock this "+
					"applier moved is already the one time.Now() reads", shifted, value)
			}
		})
	}
}
