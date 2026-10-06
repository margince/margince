// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H3

package gates

// Every workflow job carries a wall-clock ceiling.
//
// A job with no `timeout-minutes` inherits GitHub's default of SIX HOURS. That
// is not a bound, it is an outage: a required check that hangs holds the merge
// for a working day, and while it hangs it is indistinguishable from a queue
// backlog — so it is not even read as a failure while it does the damage.
//
// The case that motivated this: a stalled dependency download in the `uat` job
// sat in_progress for 2h20m against a lane that normally finishes in five
// minutes, and had to be cancelled by hand (#1836). Nothing in the product was
// wrong; nothing reported anything.
//
// Derived from the workflow tree rather than a list of job names, so a job
// added later is covered the day it is committed — which is the whole reason
// this is a fitness test and not a one-time edit. It lives beside the other
// gates that read ci.yml (laneconnbudget, frontendlaneparity,
// contractfrontendlane) rather than as a script in a fourth language.

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// workflowDir holds every workflow this repository runs. The list of FILES is
// read from disk for the same reason the job list is: a workflow added later is
// covered without anyone remembering to name it here.
const workflowDir = "../.github/workflows"

// actionDir holds this repository's own composite actions, which run steps the
// workflow files no longer spell themselves.
const actionDir = "../.github/actions"

// workflowJobs is the shape this gate needs and nothing more — decoding the
// whole GitHub schema would couple the gate to fields it does not judge.
type workflowJobs struct {
	Jobs map[string]struct {
		//nolint:tagliatelle // GitHub names this key, not us.
		TimeoutMinutes int            `yaml:"timeout-minutes"`
		Uses           string         `yaml:"uses"`
		Steps          []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

// workflowStep is one step, in a workflow job or a composite action: both
// spellings carry the same three fields this gate judges.
type workflowStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
	//nolint:tagliatelle // GitHub names this key, not us.
	TimeoutMinutes int `yaml:"timeout-minutes"`
}

// workflowFiles lists every workflow this repository runs.
//
// Both extensions, because GitHub Actions honours both. Globbing one would
// leave a caller blind to a whole class of workflow — the precise hole a
// derived check exists to not have.
func workflowFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, ext := range []string{"*.yml", "*.yaml"} {
		found, err := filepath.Glob(filepath.Join(workflowDir, ext))
		if err != nil {
			t.Fatalf("listing %s workflows: %v", ext, err)
		}
		files = append(files, found...)
	}
	// A scan over nothing reports exactly like a clean tree, which is the
	// failure mode every derived check here has to close explicitly.
	if len(files) == 0 {
		t.Fatalf("no workflows found under %s; a gate reading them would pass vacuously", workflowDir)
	}
	return files
}

// readWorkflowJobs decodes one workflow down to the jobs and steps THIS gate
// judges. Five other gates in this package decode a workflow too, each to the
// fields it reads and no further; the shared name is the reader, not the shape.
func readWorkflowJobs(t *testing.T, path string) workflowJobs {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- a repo-relative workflow path from the glob above
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var wf workflowJobs
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if len(wf.Jobs) == 0 {
		t.Fatalf("%s declares no jobs; either it is not a workflow or these gates cannot see its jobs",
			filepath.Base(path))
	}
	return wf
}

func TestEveryWorkflowJobCarriesATimeoutCeiling(t *testing.T) {
	t.Parallel()
	for _, path := range workflowFiles(t) {
		wf := readWorkflowJobs(t, path)
		for _, name := range slices.Sorted(maps.Keys(wf.Jobs)) {
			job := wf.Jobs[name]
			// A job that only CALLS a reusable workflow cannot carry a timeout
			// of its own; the called workflow's jobs own one, and they are
			// checked on their own pass through this loop.
			if job.Uses != "" && len(job.Steps) == 0 {
				continue
			}
			if job.TimeoutMinutes == 0 {
				t.Errorf("%s: job %q has no timeout-minutes, so it inherits GitHub's six-hour default — "+
					"a hang there holds a required check for a working day while reading as a queue backlog",
					filepath.Base(path), name)
			}
		}
	}
}

// The commands in this tree that install from somewhere the runner image does
// not pin. `install-deps` shells out to apt inside the runner, so it depends on
// a mirror nobody here controls, and a slow one hangs with no output at all;
// the browser download reaches Playwright's own CDN.
//
// Two entries because the install is SPLIT: one step that hangs is one fault a
// reader can name, where the combined `--with-deps` form could only report that
// something in it did (#6972).
// One prefix rather than the spellings: `playwright install`, `install-deps`,
// `install --with-deps` and `install chromium` all begin this way, and a list of
// variants would miss the next one somebody writes — a bare `playwright install`
// fetches every browser and would have escaped a list naming chromium.
var unpinnedInstalls = []string{"playwright install"}

// isUnpinnedInstall is the predicate the scan applies. The table below calls it
// rather than re-deriving the match, so a change to the list is judged by those
// cases instead of by two spellings agreeing with each other.
func isUnpinnedInstall(run string) bool {
	return slices.ContainsFunc(unpinnedInstalls, func(cmd string) bool {
		return strings.Contains(run, cmd)
	})
}

// A job ceiling bounds the damage; a step ceiling says WHERE.
//
// Without one, a stalled mirror spends the job's whole budget and reports
// "the job was cancelled" — which reads as the lane being slow, and sends the
// next contact to the change under review. The change under review is never the
// cause, because this step runs before a single test does.
// A composite action's steps cannot carry timeout-minutes, so the bound they
// use is the command. Both spellings satisfy the scan, and an install with
// neither does not.
func TestEitherSpellingOfTheBoundSatisfiesTheScan(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		step  workflowStep
		bound bool
	}{
		{"the step key", workflowStep{Run: "pnpm exec playwright install chromium", TimeoutMinutes: 6}, true},
		{"the command", workflowStep{Run: "timeout 6m pnpm exec playwright install chromium"}, true},
		{"neither", workflowStep{Run: "pnpm exec playwright install chromium"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := &testing.T{}
			boundedInstalls(probe, "action.yml", "(composite)", []workflowStep{tc.step})
			if probe.Failed() == tc.bound {
				t.Errorf("step %+v: failed=%v, want bounded=%v", tc.step, probe.Failed(), tc.bound)
			}
		})
	}
}

func TestTheUnpinnedInstallIsBoundedWhereverItRuns(t *testing.T) {
	t.Parallel()

	found := 0
	for _, path := range workflowFiles(t) {
		wf := readWorkflowJobs(t, path)
		for _, name := range slices.Sorted(maps.Keys(wf.Jobs)) {
			found += boundedInstalls(t, path, name, wf.Jobs[name].Steps)
		}
	}
	// Composite actions too: a step that moves into one leaves the workflow
	// tree, and a scan that only reads workflows would report the move as a
	// clean tree rather than as the step it stopped watching.
	for _, path := range compositeActionFiles(t) {
		found += boundedInstalls(t, path, "(composite)", readCompositeSteps(t, path))
	}
	if found == 0 {
		t.Errorf("no step in the workflow or action tree runs any of %q. Either they are gone — delete "+
			"this gate with them — or the scan stopped matching, which reads exactly like a clean tree",
			unpinnedInstalls)
	}
}

// boundedInstalls checks one step list and returns how many unpinned installs
// it held, so the caller can tell an empty corpus from a clean one.
func boundedInstalls(t *testing.T, path, job string, steps []workflowStep) int {
	t.Helper()
	found := 0
	for _, step := range steps {
		if !isUnpinnedInstall(step.Run) {
			continue
		}
		found++
		if step.TimeoutMinutes == 0 && !strings.Contains(step.Run, "timeout ") {
			t.Errorf("%s: job %q, step %q installs from an unpinned package repository with no bound "+
				"of its own, so a stalled mirror spends the job's whole budget and reports as the lane "+
				"timing out rather than as the install hanging. Either timeout-minutes on the step, or "+
				"a `timeout` command in the run — which is what a composite action's steps must use, "+
				"GitHub refusing timeout-minutes there",
				filepath.Base(path), job, step.Name)
		}
	}
	return found
}

// compositeActionFiles lists this repository's own composite actions.
func compositeActionFiles(t *testing.T) []string {
	t.Helper()
	var found []string
	// Both spellings, as the workflow glob takes both and for the same reason:
	// GitHub honours either, so reading one would leave a whole class of action
	// unscanned.
	for _, name := range []string{"action.yml", "action.yaml"} {
		matched, err := filepath.Glob(filepath.Join(actionDir, "*", name))
		if err != nil {
			t.Fatalf("listing composite actions: %v", err)
		}
		found = append(found, matched...)
	}
	if len(found) == 0 {
		t.Fatalf("no composite actions found under %s; a gate reading them would pass vacuously", actionDir)
	}
	return found
}

func readCompositeSteps(t *testing.T, path string) []workflowStep {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var action struct {
		Runs struct {
			Steps []workflowStep `yaml:"steps"`
		} `yaml:"runs"`
	}
	if err := yaml.Unmarshal(raw, &action); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return action.Runs.Steps
}

// What the scan above cannot see is a step that reaches the same mirror by
// another spelling, so the matcher is asserted in both directions on the
// spellings it must and must not answer to.
func TestTheInstallScanMatchesTheCommandAndNotItsNeighbours(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		run   string
		bound bool
	}{
		{"pnpm exec playwright install --with-deps chromium", true},
		{"pnpm install --frozen-lockfile\npnpm exec playwright install --with-deps chromium", true},
		{"npx playwright install --with-deps", true},
		// The split spellings, each an unpinned install in its own right.
		{"pnpm exec playwright install-deps chromium", true},
		{"pnpm exec playwright install chromium", true},
		// A bare install fetches every browser, and is the spelling a list of
		// variants would have missed.
		{"pnpm exec playwright install", true},
		{"pnpm install --frozen-lockfile --ignore-scripts", false},
		{"make frontend-e2e", false},
	} {
		if got := isUnpinnedInstall(tc.run); got != tc.bound {
			t.Errorf("%q: matched=%v, want %v", tc.run, got, tc.bound)
		}
	}
}
