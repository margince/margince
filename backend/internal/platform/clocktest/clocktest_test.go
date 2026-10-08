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

	assertOffsetFromWallTime(t, 0)
}

func TestTheFixtureApplierMovesTheBaseInstant(t *testing.T) {
	t.Setenv(clockskew.EnvVar, "fixture:200")

	assertOffsetFromWallTime(t, 200*24*time.Hour)
}

// The other two appliers move a clock this helper must not move again.
func TestAnAppliedClockIsNotShiftedTwice(t *testing.T) {
	for _, value := range []string{"machine:200", "database:200"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(clockskew.EnvVar, value)

			// The clock this applier moved is already the one time.Now() reads,
			// so the helper owes no offset of its own.
			assertOffsetFromWallTime(t, 0)
		})
	}
}

// assertOffsetFromWallTime brackets Now() between wall time plus want read on
// either side of it.
//
// Bracketing rather than measuring the gap: the two clock reads are not the
// same instant, and a gap compared against a threshold is a verdict about how
// busy the machine is, which is what mergegateclockbounds_test.go prohibits and
// what P3 means by real-clock flakiness. Three instants in order answer the same
// on any machine, and need no tolerance to be invented for them.
func assertOffsetFromWallTime(t *testing.T, want time.Duration) {
	t.Helper()

	earliest := time.Now().Add(want)
	got := Now(t)
	latest := time.Now().Add(want)

	if got.Before(earliest) || got.After(latest) {
		t.Fatalf("Now() = %v; want wall time plus %v, which is somewhere in [%v, %v]",
			got, want, earliest, latest)
	}
}
