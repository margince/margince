// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package clocktest gives a fixture the instant it dates itself from, and is
// the drift lane's composition root.
//
// A test that seeds a row "seven days out" and asserts how it groups is making
// a claim about arithmetic, not about the calendar. Written as
// time.Now().Add(7*24*time.Hour), though, it answers differently on a Thursday
// than on a Monday, and the day it changes its answer is a day nobody edited anything.
// Taking the base instant from here is what lets the drift lane move it.
//
// Why the read lives here: OPS-CFG-2 puts every environment read in the
// composition root, and a test binary has no cmd/ to be one, so the lane's
// root is this package, which resolves the offset through the config seam and
// hands it down. clockskew below it parses and decides; it reads nothing.
// That is the same shape platform/testdb carries for the integration lane,
// whose harnesses are the composition root of the lane they serve.
//
// It is not a Clock and it does not belong in production. A store that needs an
// injectable clock takes one as a dependency; this is for the fixture on the
// other side of the test, which has no dependency to inject into.
package clocktest

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// Skew is the offset this process was started with, read through the seam that
// holds the product's one environment read.
func Skew() (clockskew.Skew, error) {
	return clockskew.Parse(config.FromOS(clockskew.EnvVar))
}

// Now is the instant a fixture dates itself from: wall time on an ordinary
// run, and wall time plus the offset under the fixture applier.
func Now(t testing.TB) time.Time {
	t.Helper()

	skew, err := Skew()
	if err != nil {
		t.Fatalf("clocktest: %v", err)
	}
	return time.Now().Add(skew.FixtureOffset())
}
