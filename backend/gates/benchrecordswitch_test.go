// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Both bench harnesses ask the SAME variable whether to publish a record, and
// both answer only to the same value.
//
// "A record is a human's to publish" is one rule, and it is now enforced in two
// languages: Go reads MARGINCE_BENCH_RECORD in RecordingEnabled(), TypeScript
// reads it in the mobile spec's recordingEnabled(). A rule spelled on both
// sides of a wire is one item, and the failure mode of letting them drift is
// silent in the direction that matters — a scheduled job whose spec no longer
// consults the variable writes a runner's numbers into the published page and
// reports success, because writing the record is not what the run is judged on.
//
// The VALUE is checked as well as the variable, and that half is the one worth
// having. `bench-perf-check` and `bench-mobile-check` both CLEAR the variable
// (`MARGINCE_BENCH_RECORD=`) rather than leaving it unset, precisely so that
// "writes nothing" survives a developer who has it exported from an earlier
// by-hand run. A reader that tested "is it set" instead of "is it 1" would
// treat the cleared value as permission to write, and the two targets would
// mean the opposite of what they say.
//
// WHAT THIS CATCHES: either side ceasing to read the variable, and either side
// accepting a value the other does not.
//
// WHAT THIS DOES NOT CATCH, deliberately: whether a given make target sets the
// variable correctly. That is a property of the Makefile rather than of the two
// harnesses, and it is stated where it is read — a gate over recipe text would
// be a second copy of the recipes.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The two readers. Go's is the original; the TypeScript one is the deliberate
// mirror, so it is named second in every failure message.
const (
	goRecordReader = "internal/compose/integration/perfrecord.go"
	tsRecordReader = "../frontend/e2e/perf-mobile.spec.ts"

	// The variable both must consult, spelled once here so a rename that
	// updated only one reader fails rather than passing on each side's own
	// spelling.
	//
	// Held by: TestBothBenchHarnessesReadOneRecordingSwitch (backend/gates/benchrecordswitch_test.go)
	recordSwitch = "MARGINCE_BENCH_RECORD"
)

// recordComparison matches a comparison of the switch against a quoted value in
// either language: Go's `os.Getenv("X") == "1"` and TypeScript's
// `process.env.X === "1"`. Both the OPERATOR and the value are captured, so the
// two ways this can drift are reported as what they are.
//
// The operator is captured rather than pinned to equality, and that is the
// lesson of writing this gate: an inverted reader (`!== ""`) is a reader that
// publishes on precisely the cleared value both check targets set, and matching
// only `==` reported it as "does not consult the switch at all". True that
// nothing matched; false about the tree, and it sends a reader hunting for a
// missing read that is right there.
//
// The variable is matched in both the indexed and the dotted spelling because
// TypeScript admits `process.env.X` and `process.env["X"]`, and a reader written
// the other way would otherwise parse as nothing — an unparsed reader is one
// this gate silently agrees with, which is the failure direction a parity check
// must not have.
var recordComparison = regexp.MustCompile(
	`(?:os\.Getenv\(\s*"` + recordSwitch + `"\s*\)|process\.env(?:\.` + recordSwitch + `|\[\s*"` + recordSwitch + `"\s*\]))\s*(!?={2,3})\s*"([^"]*)"`)

func TestBothBenchHarnessesReadOneRecordingSwitch(t *testing.T) {
	t.Parallel()
	values := map[string]string{}
	for _, path := range []string{goRecordReader, tsRecordReader} {
		b, err := os.ReadFile(path) // #nosec G304 -- path is a compile-time literal
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		matches := recordComparison.FindAllStringSubmatch(string(b), -1)
		if len(matches) == 0 {
			t.Errorf("%s does not compare %s against a literal value, so it no longer decides whether to "+
				"publish a record the way the other harness does: a scheduled run of this lane would write a "+
				"runner's numbers into the published page and still report success", path, recordSwitch)
			continue
		}
		for _, m := range matches {
			operator, value := m[1], m[2]
			// A negated reader publishes on everything the value is NOT, which
			// includes the empty string the check targets deliberately set.
			if strings.HasPrefix(operator, "!") {
				t.Errorf("%s publishes a record when %s is %s%q, so the cleared value both check targets set "+
					"(`%s=`) reads as permission to write — the one case those targets exist to refuse",
					path, recordSwitch, operator, value, recordSwitch)
			}
			values[path] = value
		}
	}
	if len(values) != 2 {
		return // Already reported above; comparing one side to nothing says nothing.
	}
	if values[goRecordReader] != values[tsRecordReader] {
		t.Errorf("the two bench harnesses publish a record on different values of %s: %s wants %q, %s wants %q. "+
			"Both check targets CLEAR the variable rather than unsetting it, so a reader that accepts the empty "+
			"value treats a `writes nothing` target as permission to write",
			recordSwitch, goRecordReader, values[goRecordReader], tsRecordReader, values[tsRecordReader])
	}
}
