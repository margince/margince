// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package clockskew reads the drift lane's offset: how far the suite is being
// run from the real date, and which of the three machines in the test rig was
// moved to put it there.
//
// The backend suite's verdict must not depend on the calendar. A fixture
// correct the day it is written — "seven days out, so it groups as later" — is
// wrong on some later Thursday, and nothing in a diff causes that, so no
// pull-request gate can find it. The lane that can is a second RUN at a moved
// clock, which is what this offset arms.
//
// WHY ONE VARIABLE SPELLS BOTH HALVES. Three things can supply the shift and
// they do not compose: the host wall clock, a shadowed now() in the test
// database, and the base instant fixtures seed from. Two of them armed at once
// runs the suite at twice the offset, with the layers disagreeing about which
// day it is — failures that are the LANE's fault, which is the one outcome a
// drift lane must not have, because it teaches a reader to ignore the red. A
// single value naming the applier AND the amount makes that state unspellable
// rather than possible and policed.
package clockskew

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvVar carries `<applier>:<days>`, or nothing at all on an ordinary run.
// Exported so the lanes and the gate do not each spell the string, which is how
// a configuration surface drifts from the one that is documented.
const EnvVar = "BACKEND_CLOCK_SKEW"

// Applier is the thing that was moved to put the suite in the future.
type Applier string

// The three appliers, and the absence of one.
//
// They are not interchangeable and the difference is not a preference:
//
//   - Machine moves the host's wall clock, which is what CI does. Linux has no
//     CLOCK_REALTIME namespace — a container cannot hold a wall clock of its
//     own — so the compose Postgres reads the same moved clock the Go process
//     does, and the two cannot disagree. That is the only applier that shifts
//     every reading of now(), including the ones written by a column DEFAULT.
//   - Database shadows now() in the test template, for a laptop where moving
//     the host clock is not acceptable. It is strictly weaker: see
//     ShadowLimits.
//   - Fixture moves only the base instant tests seed from. It shifts no
//     database reading at all, so it proves a fixture's own arithmetic and
//     nothing about a row Postgres stamped.
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

// FixtureOffset is what a fixture's base instant must add to time.Now().
//
// Nonzero under Fixture alone. Under Machine the operating system has already
// moved, and under Database the database has; a helper adding its own offset on
// top would put the fixtures a further 200 days from the rows they are being
// compared against, and every such failure would be the lane's own.
func (s Skew) FixtureOffset() time.Duration {
	if s.Applier != Fixture {
		return 0
	}
	return time.Duration(s.Days) * 24 * time.Hour
}

// Armed reports whether the suite is running at a moved clock at all.
func (s Skew) Armed() bool { return s.Applier != None }

// FromEnv reads the offset this process was started with.
//
// An unset variable is the ordinary run and not an error. An unparsable one IS
// an error, and the callers fail on it rather than continuing: a typo that
// silently shifted nothing would leave the lane reporting PASS over a suite it
// never moved, which reads exactly like a suite that has no date-fragile
// fixtures left. That is the shape of failure a drift lane exists to remove.
func FromEnv() (Skew, error) { return Parse(os.Getenv(EnvVar)) }

// Parse reads `<applier>:<days>`.
func Parse(value string) (Skew, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Skew{Applier: None}, nil
	}

	applier, days, ok := strings.Cut(value, ":")
	if !ok {
		return Skew{}, fmt.Errorf("%s=%q: want <applier>:<days>, one of %s — the applier is named "+
			"beside the amount so that two of them cannot be armed at once", EnvVar, value, appliers())
	}

	parsed := Applier(strings.TrimSpace(applier))
	switch parsed {
	case Machine, Database, Fixture:
	default:
		return Skew{}, fmt.Errorf("%s=%q: unknown applier %q, want one of %s", EnvVar, value, applier, appliers())
	}

	n, err := strconv.Atoi(strings.TrimSpace(days))
	if err != nil {
		return Skew{}, fmt.Errorf("%s=%q: %q is not a number of days", EnvVar, value, days)
	}
	if n <= 0 {
		return Skew{}, fmt.Errorf("%s=%q: %d days moves nothing — an offset that shifts no clock "+
			"reports a verdict about the ordinary suite under the drift lane's name", EnvVar, value, n)
	}
	return Skew{Applier: parsed, Days: n}, nil
}

// String renders the offset the way the variable spells it, so a lane's log
// line and the value a reader would set to reproduce it are the same text.
func (s Skew) String() string {
	if !s.Armed() {
		return string(None)
	}
	return fmt.Sprintf("%s:%d", s.Applier, s.Days)
}

// ShadowLimits states what the Database applier cannot move, for the lane that
// runs under it to print. The two holes are not the same kind of hole, which is
// measurable and was measured:
//
//   - CURRENT_TIMESTAMP is a reserved keyword the parser resolves without
//     consulting search_path. Nothing closes this one.
//   - A column DEFAULT binds whatever now() resolved to WHEN THE DDL RAN. The
//     migrations build the template before any shadow exists, so their defaults
//     keep the real date — a default written after the shadow does shift. It
//     could be closed by shadowing before migrating, at the price of a template
//     whose DDL no longer says what production's says, which is a worse thing
//     for a reproduction aid to carry than a known-weaker shift.
//
// A reader who does not know this reads a green Database run as the verdict a
// Machine run gives. It is not one, and saying so is the difference between a
// reproduction aid and a lane whose red means nothing.
const ShadowLimits = "CURRENT_TIMESTAMP and stored column DEFAULTs keep the real date under the database applier"

// appliers renders the accepted vocabulary for an error a reader has to act on.
func appliers() string {
	return fmt.Sprintf("%q, %q, %q", Machine, Database, Fixture)
}
