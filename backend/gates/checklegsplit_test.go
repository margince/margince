// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// CI splits the backend gate's fan-out over two jobs: deterministic-gates runs
// `make check-backend CHECK_LEGS="…"` and backend-lint runs `make check-static`.
// A leg that neither names still passes `ci`, because nothing ran it. This test
// holds the two jobs to the full CHECK_LEGS list a local `make check` runs.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	backendMakefile = "Makefile"
	testLegsJob     = "deterministic-gates"
	staticLegsJob   = "backend-lint"
)

// ciLegsOverride is the CHECK_LEGS value a `make check-backend` step passes.
var ciLegsOverride = regexp.MustCompile(`make check-backend\b[^\n]*\bCHECK_LEGS="([^"]*)"`)

type stepRuns struct {
	Jobs map[string]struct {
		Needs stringList     `yaml:"needs"`
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

func TestTheTwoBackendJobsRunEveryCheckLeg(t *testing.T) {
	t.Parallel()
	all, static := backendCheckLegs(t)

	raw, err := os.ReadFile(aggregateWorkflow) // #nosec G304 -- a fixed repo-relative path
	if err != nil {
		t.Fatalf("reading %s: %v", aggregateWorkflow, err)
	}
	var wf stepRuns
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", aggregateWorkflow, err)
	}
	testLegs, ok := legsPassedBy(wf, testLegsJob)
	if !ok {
		t.Fatalf("job %q has no `make check-backend` step, so this test cannot tell which legs it runs", testLegsJob)
	}
	if testLegs == nil {
		testLegs = all
	}
	if !jobRuns(wf, staticLegsJob, "make check-static") {
		t.Fatalf("job %q has no `make check-static` step, so nothing in CI runs the legs %v", staticLegsJob, static)
	}

	for _, job := range []string{testLegsJob, staticLegsJob} {
		if !slices.Contains(wf.Jobs[aggregateJob].Needs, job) {
			t.Errorf("job %q is not in %q's needs, so the required check passes whatever its legs report", job, aggregateJob)
		}
	}

	ran := append(slices.Clone(testLegs), static...)
	for _, leg := range all {
		if !slices.Contains(ran, leg) {
			t.Errorf("CHECK_LEGS leg %q runs in a local `make check` but in neither %s (CHECK_LEGS=%q) nor %s (%v). "+
				"Add it to one of them, or `ci` passes without it.", leg, testLegsJob, strings.Join(testLegs, " "), staticLegsJob, static)
		}
	}
}

// backendCheckLegs reads CHECK_LEGS and the legs check-static runs, both
// expanded through the Makefile's literal variables.
func backendCheckLegs(t *testing.T) (all, static []string) {
	t.Helper()
	body, err := os.ReadFile(backendMakefile)
	if err != nil {
		t.Fatalf("reading %s: %v", backendMakefile, err)
	}
	vars := readLiteralVars(string(body))
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if match := makeAssignment.FindStringSubmatch(line); match != nil && match[1] == "CHECK_LEGS" {
			all = expandTokens(strings.Fields(match[3]), vars)
		}
		if strings.HasPrefix(line, "check-static:") && i+1 < len(lines) {
			static, _ = parseRecipe(t, lines[i+1], vars)
		}
	}
	if len(all) == 0 || len(static) == 0 {
		t.Fatalf("%s: read CHECK_LEGS=%v and check-static legs=%v; an empty side would pass this test without checking anything", backendMakefile, all, static)
	}
	for _, leg := range append(slices.Clone(all), static...) {
		if strings.ContainsAny(leg, "$()") {
			t.Fatalf("%s: leg %q is still a variable reference after expansion, so its legs are unknown", backendMakefile, leg)
		}
	}
	return all, static
}

// legsPassedBy answers the CHECK_LEGS a job's `make check-backend` step passes,
// and nil when the step passes none, which runs every leg.
func legsPassedBy(wf stepRuns, job string) ([]string, bool) {
	for _, step := range wf.Jobs[job].Steps {
		if !strings.Contains(step.Run, "make check-backend") {
			continue
		}
		if match := ciLegsOverride.FindStringSubmatch(step.Run); match != nil {
			return strings.Fields(match[1]), true
		}
		return nil, true
	}
	return nil, false
}

func jobRuns(wf stepRuns, job, command string) bool {
	return slices.ContainsFunc(wf.Jobs[job].Steps, func(step workflowStep) bool {
		return strings.Contains(step.Run, command)
	})
}
