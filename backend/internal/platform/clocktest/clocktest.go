// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package clocktest gives a fixture the instant it dates itself from, and is
// the drift lane's composition root.
//
// A test that seeds a row "seven days out" and asserts how it groups makes a
// claim about arithmetic, not the calendar. Written as
// time.Now().Add(7*24*time.Hour), it answers differently on a Thursday than on
// a Monday. Taking the base instant from here lets the drift lane move it.
//
// Why the read lives here: OPS-CFG-2 puts every environment read in the
// composition root. A test binary has no cmd/ to be one, so this package is the
// lane's root. It resolves the offset through the config seam and hands it
// down; clockskew below it parses and decides, and reads nothing. platform/testdb
// has the same shape for the integration lane.
//
// It is not a Clock and does not belong in production. A store that needs an
// injectable clock takes one as a dependency. This is for the fixture on the
// other side of the test, which has no dependency to inject into.
package clocktest

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/config/osenv"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// Skew is the offset this process was started with, read through the seam that
// holds the product's one environment read.
func Skew() (clockskew.Skew, error) {
	return clockskew.Parse(osenv.Get(clockskew.EnvVar))
}

// Now is the instant a fixture dates itself from: wall time on an ordinary
// run, and wall time plus the offset under the fixture applier. It is cut to
// the microsecond Postgres stores, so a fixture date reads back equal to the
// value it wrote.
func Now(t testing.TB) time.Time {
	t.Helper()

	skew, err := Skew()
	if err != nil {
		t.Fatalf("clocktest: %v", err)
	}
	return time.Now().Add(skew.FixtureOffset()).Truncate(time.Microsecond)
}
