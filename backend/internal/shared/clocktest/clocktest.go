// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package clocktest gives a fixture the instant it dates itself from.
//
// A test that seeds a row "seven days out" and asserts how it groups is making
// a claim about arithmetic, not about the calendar — but written as
// time.Now().Add(7*24*time.Hour) it answers differently on a Thursday than on a
// Monday, and the day it changes its answer is a day nobody edited anything.
// Taking the base instant from here is what lets the drift lane move it.
//
// It is not a Clock and it does not belong in production. A store that needs an
// injectable clock takes one as a dependency; this is for the fixture on the
// other side of the test, which has no dependency to inject into. gatekit holds
// the same line for waivers, and for the same reason: a test-only mechanism
// reachable from product code becomes a way to move the product's clock.
package clocktest

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// Now is the instant a fixture dates itself from — wall time on an ordinary
// run, and wall time plus the offset under the fixture applier.
//
// It fails the test on an unparsable offset rather than falling back to wall
// time. A drift lane that quietly ran unshifted would report PASS over a suite
// it never moved, and that reads exactly like a suite with nothing left to find.
func Now(t testing.TB) time.Time {
	t.Helper()

	skew, err := clockskew.FromEnv()
	if err != nil {
		t.Fatalf("clocktest: %v", err)
	}
	return time.Now().Add(skew.FixtureOffset())
}
