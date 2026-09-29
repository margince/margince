// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The contract's clock pattern and this package's parser are one invariant with
// two spellings, and a generated client validates against the pattern before
// the parser ever sees the request. When they disagree the client refuses a
// time the server would have kept, and no test of either side alone can see it.

import (
	"os"
	"regexp"
	"testing"
)

const workingHoursContract = "../../../api/crm.yaml"

// clockPattern reads one named field's `pattern:` out of the WorkingHours
// schema. The field name is required in the match so a pattern moved to a
// neighbouring property cannot answer for this one — a census that reads the
// wrong line reports PASS, which is the one way this must not fail.
func clockPattern(t *testing.T, field string) *regexp.Regexp {
	t.Helper()
	source, err := os.ReadFile(workingHoursContract)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	find := regexp.MustCompile(`(?m)^[ \t]+` + regexp.QuoteMeta(field) +
		`:\n[ \t]+type: string\n[ \t]+pattern: '([^']+)'`)
	match := find.FindSubmatch(source)
	if match == nil {
		t.Fatalf("%s declares no `type: string` with a pattern in %s; if the "+
			"contract moved the field, move this gate with it",
			field, workingHoursContract)
	}
	compiled, err := regexp.Compile(string(match[1]))
	if err != nil {
		t.Fatalf("the %s pattern does not compile: %v", field, err)
	}
	return compiled
}

func TestTheContractsClockPatternAcceptsWhatTheServerReads(t *testing.T) {
	// start_time stops at 23:59; only end_time spells the end of the day.
	for _, field := range []string{"start_time", "end_time"} {
		pattern := clockPattern(t, field)
		t.Run(field, func(t *testing.T) {
			for written := range clockTimesAccepted {
				if written == "24:00" && field == "start_time" {
					if pattern.MatchString(written) {
						t.Errorf("start_time accepts %q; a day does not start at midnight's end", written)
					}
					continue
				}
				if !pattern.MatchString(written) {
					t.Errorf("the %s pattern refuses %q, which the server reads as a time", field, written)
				}
			}
			for _, written := range clockTimesRefused {
				if pattern.MatchString(written) {
					t.Errorf("the %s pattern accepts %q, which the server refuses", field, written)
				}
			}
		})
	}
}
