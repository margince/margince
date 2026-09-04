// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// That the daily reading is actually PLACED, and reads the window it says.

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// THE SCHEDULE HAS TO REACH THE RUNNER, which is a different claim from the
// cadence being declared.
//
// A pass whose worker is wired and whose schedule is not runs never, and every
// symptom of that is silence: no job row, no error, no log line. That is not
// hypothetical — this pass was written with its schedule returned from a helper
// that wireJobs did not call, and the dev stack simply never produced a report
// while every sibling pass ran normally.
//
// It reads the SOURCE because river.PeriodicJob has only unexported fields:
// nothing in the built slice can be asked which kind it carries, so a test over
// the returned value can only count schedules and would pass with this one
// missing. An earlier draft did exactly that, and the mutation below survived
// it.
//
// Mutation: drop the periodicFor line from wireJobs and this fails.
func TestTheDisagreementReadingIsScheduled(t *testing.T) {
	t.Parallel()

	if got := periodicFor(JobRunnerConfig{}, AuthzDisagreementArgs{}); len(got) != 1 {
		t.Fatalf("periodicFor placed %d schedules for the disagreement reading, want 1", len(got))
	}

	source, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatalf("reading the wiring: %v", err)
	}
	placement := "periodicFor(cfg, AuthzDisagreementArgs{})"
	if !strings.Contains(string(source), placement) {
		t.Errorf("jobs.go does not place %s: the worker is wired and the pass runs never, "+
			"and every symptom of that is silence", placement)
	}
	// Under-recognition: if the placement spelling changed wholesale, the check
	// above would report a missing schedule that is present. Every sibling
	// placement uses this form, so finding none means the file was restructured
	// and this test has to be rewritten rather than believed.
	if strings.Count(string(source), "periodicFor(cfg, ") < 2 {
		t.Fatal("jobs.go holds fewer than two periodicFor placements: the wiring has been " +
			"restructured and this test is reading for a form that no longer exists")
	}
}

// THE WINDOW MATCHES THE CADENCE, and slightly exceeds it.
//
// Consecutive passes must describe consecutive periods: a window shorter than
// the cadence leaves minutes no reading ever covers, and a longer one reports
// one disagreement on several days running, which reads as a growing problem
// when it is one problem counted repeatedly. The excess is what absorbs a pass
// that starts late.
//
// Mutation: set disagreementWindow below the cadence and this fails.
func TestTheWindowCoversTheCadenceItRunsOn(t *testing.T) {
	t.Parallel()

	spec, declared := jobs.SpecFor(AuthzDisagreementArgs{}.Kind())
	if !declared {
		t.Fatalf("api/jobs.yaml does not declare %q", AuthzDisagreementArgs{}.Kind())
	}
	cadence := spec.Cadence.Fixed
	if cadence <= 0 {
		t.Fatalf("the disagreement reading declares no fixed cadence (%v): the window below has "+
			"nothing to be measured against", cadence)
	}
	if disagreementWindow < cadence {
		t.Errorf("the window is %v and the cadence %v: the minutes between them are read by no "+
			"pass at all", disagreementWindow, cadence)
	}
	// An unbounded excess would defeat the point of a window. A day's slack on a
	// daily pass is the intent; a week's would report the same disagreement
	// seven times.
	if slack := disagreementWindow - cadence; slack > cadence {
		t.Errorf("the window exceeds the cadence by %v, more than the cadence itself: each pass "+
			"re-reports what the last one already did", slack)
	}
}
