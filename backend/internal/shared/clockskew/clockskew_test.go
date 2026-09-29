// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package clockskew

import (
	"testing"
	"time"
)

func TestAnUnsetVariableIsTheOrdinaryRun(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "   "} {
		skew, err := Parse(value)
		if err != nil {
			t.Fatalf("Parse(%q) = %v; an unset offset is the ordinary suite, not an error", value, err)
		}
		if skew.Armed() || skew.FixtureOffset() != 0 {
			t.Fatalf("Parse(%q) = %+v; want no applier and no offset", value, skew)
		}
	}
}

func TestAnOffsetNamesItsApplierAndItsAmount(t *testing.T) {
	t.Parallel()

	want := map[string]Skew{
		"machine:200":   {Applier: Machine, Days: 200},
		"database:200":  {Applier: Database, Days: 200},
		"fixture:1":     {Applier: Fixture, Days: 1},
		" machine : 7 ": {Applier: Machine, Days: 7},
	}
	for value, expected := range want {
		skew, err := Parse(value)
		if err != nil {
			t.Fatalf("Parse(%q) = %v; want %+v", value, err, expected)
		}
		if skew != expected {
			t.Fatalf("Parse(%q) = %+v; want %+v", value, skew, expected)
		}
	}
}

// A value that shifts nothing must not parse. The lane would otherwise run the
// ordinary suite and report its verdict under the drift lane's name, which
// reads identically to a suite with no date-fragile fixtures left.
func TestAValueThatMovesNoClockIsRefused(t *testing.T) {
	t.Parallel()

	refused := []string{
		"200",          // the amount without an applier
		"machine",      // the applier without an amount
		"machine:",     // ditto, with the separator
		"machine:none", // an amount that is not a number
		"machine:0",    // an offset of nothing
		"machine:-200", // backwards, which no lane asks for
		"clock:200",    // an applier that does not exist
		"MACHINE:200",  // the vocabulary is lower case
	}
	for _, value := range refused {
		if skew, err := Parse(value); err == nil {
			t.Errorf("Parse(%q) = %+v, nil; want an error — a value that arms nothing must fail "+
				"rather than quietly run the ordinary suite", value, skew)
		}
	}
}

// The coherence rule the whole design rests on: exactly one layer supplies the
// shift. Under machine the operating system already moved and under database
// the database did, so a fixture helper adding its own would run the suite at
// twice the offset with the layers disagreeing.
func TestOnlyTheFixtureApplierMovesAFixturesBaseInstant(t *testing.T) {
	t.Parallel()

	want := map[Applier]time.Duration{
		Fixture:  200 * 24 * time.Hour,
		Machine:  0,
		Database: 0,
		None:     0,
	}
	for applier, offset := range want {
		got := Skew{Applier: applier, Days: 200}.FixtureOffset()
		if got != offset {
			t.Errorf("Skew{%q, 200}.FixtureOffset() = %v; want %v", applier, got, offset)
		}
	}
}

// A lane prints the offset it ran at, and a reader reproduces it by setting
// that text. The two are the same rendering or the instruction is wrong.
func TestTheRenderingIsTheValueThatReproducesIt(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"machine:200", "database:14", "fixture:1"} {
		skew, err := Parse(value)
		if err != nil {
			t.Fatalf("Parse(%q) = %v", value, err)
		}
		if skew.String() != value {
			t.Errorf("Parse(%q).String() = %q; want the value back", value, skew.String())
		}
		again, err := Parse(skew.String())
		if err != nil || again != skew {
			t.Errorf("Parse(%q.String()) = %+v, %v; want %+v", value, again, err, skew)
		}
	}
	if unarmed := (Skew{}).String(); unarmed != "" {
		t.Errorf("Skew{}.String() = %q; want the empty value an unset variable holds", unarmed)
	}
}

func TestTheProcessReadsItsOffsetFromTheEnvironment(t *testing.T) {
	t.Setenv(EnvVar, "fixture:200")

	skew, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() = %v; want the offset the process was started with", err)
	}
	if skew.FixtureOffset() != 200*24*time.Hour {
		t.Fatalf("FromEnv() = %+v; want 200 days of fixture offset", skew)
	}
}
