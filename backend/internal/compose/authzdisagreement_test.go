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

// A wired worker without its periodic placement never runs. Inspect source
// because River does not expose a schedule’s job kind.
func TestTheDisagreementReadingIsScheduled(t *testing.T) {
	t.Parallel()

	if got := periodicFor(JobRunnerConfig{}, AuthzDisagreementArgs{}); len(got) != 1 {
		t.Fatalf("periodicFor placed %d schedules for the disagreement reading, want 1", len(got))
	}

	wiring, err := os.ReadFile("jobs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wiring), "placedSchedules(cfg)") {
		t.Fatal("the runner does not use its base schedules")
	}
	source, err := os.ReadFile("jobschedule.go")
	if err != nil {
		t.Fatalf("reading the wiring: %v", err)
	}
	placement := "periodicFor(cfg, AuthzDisagreementArgs{})"
	if !strings.Contains(string(source), placement) {
		t.Errorf("jobschedule.go does not place %s: the worker is wired and the pass runs never, "+
			"and every symptom of that is silence", placement)
	}
	// Require the current placement spelling so a renamed schedule cannot pass vacuously.
	if strings.Count(string(source), "periodicFor(cfg, ") < 2 {
		t.Fatal("jobschedule.go holds fewer than two periodicFor placements: the wiring has been " +
			"restructured and this test is reading for a form that no longer exists")
	}
}

// The reading window must cover the cadence and its scheduling delay.
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
