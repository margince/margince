// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package clockskew reads the drift lane's offset. It says how far from the real
// date the suite runs, and which of the test rig's three clocks moved.
//
// gates/wallclockfixtures_test.go carries why the lane exists. This package
// parses the value and reads nothing itself.
package clockskew

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// EnvVar carries `<applier>:<days>`, or nothing at all on an ordinary run.
const EnvVar = "BACKEND_CLOCK_SKEW"

// maxDays bounds the offset at a century. A day count near the int64 nanosecond
// ceiling wraps FixtureOffset to a large negative duration. The suite would then
// run in the past under the drift lane's name. The zero check refuses the same
// defect from the other end.
const maxDays = 36500

// Applier is the clock that was moved to put the suite in the future.
type Applier string

// The three appliers, and the absence of one. They are not interchangeable:
//
//   - Machine moves the host's wall clock, which is what CI does. The Go
//     process and the compose Postgres read that one clock, so it reaches every
//     reading of now(); scripts/clock-drift-host.sh says why no other shift can.
//   - Database shadows now() in the test template, for a machine whose wall
//     clock must not move. Strictly weaker: platform/testdb/clockshadow.go
//     states what it cannot reach.
//   - Fixture moves only the base instant tests seed from, through
//     platform/clocktest. It shifts no database reading at all.
const (
	None     Applier = ""
	Machine  Applier = "machine"
	Database Applier = "database"
	Fixture  Applier = "fixture"
)

// Skew is the parsed offset: how far, and applied by whom.
type Skew struct {
	Applier Applier
	Days    int
}

// FixtureOffset is what a fixture's base instant adds to time.Now().
//
// Nonzero under Fixture alone. Under Machine the operating system has already
// moved, and under Database the database has. A helper adding its own on top
// would put the fixtures a second offset away from the rows they meet. Those
// failures would belong to the lane, not the tree.
func (s Skew) FixtureOffset() time.Duration {
	if s.Applier != Fixture {
		return 0
	}
	return time.Duration(s.Days) * 24 * time.Hour
}

// Armed reports whether the suite is running at a moved clock at all.
func (s Skew) Armed() bool { return s.Applier != None }

// Parse reads `<applier>:<days>`. An unset value is the ordinary run.
//
// Anything else that cannot be read is an error, not a fallback to the ordinary
// run. A typo that shifted nothing would leave the lane reporting PASS over a
// suite it never moved.
//
// The applier is named beside the amount so that two cannot be armed at once.
// The three clocks do not compose. Two armed together run the suite at twice the
// offset, with the layers disagreeing about which day it is.
func Parse(value string) (Skew, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Skew{Applier: None}, nil
	}

	applier, days, ok := strings.Cut(value, ":")
	if !ok {
		return Skew{}, fmt.Errorf("%s=%q: want <applier>:<days>, one of %s", EnvVar, value, appliers())
	}

	parsed := Applier(strings.TrimSpace(applier))
	switch parsed {
	case Machine, Database, Fixture:
	default:
		return Skew{}, fmt.Errorf("%s=%q: unknown applier %q, want one of %s", EnvVar, value, applier, appliers())
	}

	n, err := strconv.Atoi(strings.TrimSpace(days))
	switch {
	case err != nil:
		return Skew{}, fmt.Errorf("%s=%q: %q is not a number of days", EnvVar, value, days)
	case n <= 0:
		return Skew{}, fmt.Errorf("%s=%q: %d days moves nothing — an offset that shifts no clock "+
			"reports a verdict about the ordinary suite under the drift lane's name", EnvVar, value, n)
	case n > maxDays:
		return Skew{}, fmt.Errorf("%s=%q: %d days is past the %d-day ceiling, beyond which the offset "+
			"wraps negative and runs the suite in the past", EnvVar, value, n, maxDays)
	}
	return Skew{Applier: parsed, Days: n}, nil
}

// String renders the offset the way the variable spells it. A lane's log line is
// then the value a reader sets to reproduce it.
func (s Skew) String() string {
	if !s.Armed() {
		return string(None)
	}
	return fmt.Sprintf("%s:%d", s.Applier, s.Days)
}

func appliers() string {
	return fmt.Sprintf("%q, %q, %q", Machine, Database, Fixture)
}
