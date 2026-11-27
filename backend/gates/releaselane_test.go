// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// The release scripts are tested by `make check-release`, not by `make check`.
// Only the release workflows run those scripts, so a test of one costs every
// other change a run and gains it nothing. The lane must then hold in three
// places, each derived here from the tree:
//
//   - every release script test is in `check-release`, and none is in `check`;
//   - the CI job that runs the lane runs on a change to any file those tests
//     read, and to any workflow that calls a release script;
//   - a release workflow runs the lane before any job of it calls a release
//     script, so a release never trusts a pull request's result alone.

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	releaseLane      = "check-release"
	releaseTestGlob  = "../scripts/release*.test.sh"
	workflowGlob     = "../.github/workflows/*.yml"
	releaseLaneMake  = "make " + releaseLane
	minReleaseTests  = 10
	minReleaseCalls  = 3
	scriptsDirPrefix = "scripts/"
)

// A release script as a workflow step or a test names it: `scripts/release-x.sh`
// or `scripts/release_notes.py`.
var releaseScriptCall = regexp.MustCompile(`scripts/release[-_][\w.-]+`)

// A file a test script opens next to itself: `$SCRIPT_DIR/release-x.sh`.
var siblingScript = regexp.MustCompile(`\$\{?SCRIPT_DIR\}?/([\w.-]+)`)

// releaseTests lists the release script tests in the tree, repository-relative.
func releaseTests(t *testing.T) []string {
	t.Helper()
	paths, err := gatekit.Glob(releaseTestGlob)
	if err != nil {
		t.Fatalf("listing %s: %v", releaseTestGlob, err)
	}
	if len(paths) < minReleaseTests {
		t.Fatalf("%s matched %d test(s), and the tree has more than %d — the census stopped finding them",
			releaseTestGlob, len(paths), minReleaseTests)
	}
	tests := make([]string, 0, len(paths))
	for _, path := range paths {
		tests = append(tests, strings.TrimPrefix(path, "../"))
	}
	return tests
}

func TestEveryReleaseScriptTestRunsInCheckReleaseAndNotInCheck(t *testing.T) {
	t.Parallel()
	targets, vars := parseRecipes(t, rootMakefile)
	inRelease := scriptsUnder(targets, vars, releaseLane)
	inCheck := scriptsUnder(targets, vars, "check")

	// A walk from `check` that misses the root gates finds no release test.
	// The second half below would then pass having read nothing.
	rootGates := scriptsUnder(targets, vars, "check-root-gates")
	if len(rootGates) == 0 {
		t.Fatalf("the walk from `make check-root-gates` found no script — the recipe reading broke")
	}
	for _, script := range rootGates {
		if !slices.Contains(inCheck, script) {
			t.Fatalf("the walk from `make check` misses %s, which check-root-gates runs — it cannot tell what `make check` runs", script)
		}
	}

	for _, test := range releaseTests(t) {
		if !slices.Contains(inRelease, test) {
			t.Errorf("no target under `make %s` runs %s. Add its target to RELEASE_SCRIPT_GATES in the root Makefile.",
				releaseLane, test)
		}
		if slices.Contains(inCheck, test) {
			t.Errorf("`make check` runs %s. Only the release workflows run release scripts, so their tests belong in "+
				"RELEASE_SCRIPT_GATES, not ROOT_SCRIPT_GATES.", test)
		}
	}
}

// releaseLaneInputs lists the paths that must trigger `make check-release`.
// These are the scripts the Makefile names, the files each test opens next to
// itself, and the workflows that call a release script.
func releaseLaneInputs(t *testing.T, lane filteredLane) []string {
	t.Helper()
	inputs := map[string]bool{}
	for _, input := range lane.inputs {
		inputs[input] = true
		if !strings.HasSuffix(input, ".test.sh") {
			continue
		}
		body, err := gatekit.ReadFile(filepath.Join("..", input))
		if err != nil {
			t.Fatalf("reading %s: %v", input, err)
		}
		for _, match := range siblingScript.FindAllStringSubmatch(string(body), -1) {
			sibling := scriptsDirPrefix + match[1]
			if _, err := gatekit.ReadFile(filepath.Join("..", sibling)); err == nil {
				inputs[sibling] = true
			}
		}
	}
	for _, workflow := range releaseWorkflows(t) {
		inputs[workflow] = true
	}
	return sorted(inputs)
}

// releaseWorkflows lists the workflows whose text calls a release script.
func releaseWorkflows(t *testing.T) []string {
	t.Helper()
	paths, err := gatekit.Glob(workflowGlob)
	if err != nil {
		t.Fatalf("listing %s: %v", workflowGlob, err)
	}
	var found []string
	for _, path := range paths {
		body, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if releaseScriptCall.Match(body) {
			found = append(found, strings.TrimPrefix(path, "../"))
		}
	}
	return found
}

func TestTheReleaseLaneRunsOnAChangeToWhatItTests(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, classifierWorkflow)
	scopes := filterScopes(t, workflow)
	var runners []filteredLane
	for _, lane := range filteredLanes(t, workflow) {
		commands := jobCommands(t, workflow, workflow.Jobs[lane.name].Uses, lane.name)
		if strings.Contains(strings.Join(commands, "\n"), releaseLaneMake) {
			runners = append(runners, lane)
		}
	}
	if len(runners) != 1 {
		t.Fatalf("%d job(s) in %s guarded by a scope run `%s`, want one", len(runners), classifierWorkflow, releaseLaneMake)
	}
	lane := runners[0]
	inputs := releaseLaneInputs(t, lane)

	// The two kinds of input laneinputscope cannot see must each be found, or
	// this test passes having added nothing to it.
	var workflows, siblings int
	for _, input := range inputs {
		switch {
		case strings.HasPrefix(input, ".github/workflows/"):
			workflows++
		case strings.HasPrefix(input, scriptsDirPrefix) && !slices.Contains(lane.inputs, input):
			siblings++
		}
	}
	if workflows < minReleaseCalls || siblings < minReleaseTests {
		t.Fatalf("found %d release workflow(s) and %d script(s) the release tests open; the tree has at least %d and %d",
			workflows, siblings, minReleaseCalls, minReleaseTests)
	}

	lane.inputs = inputs
	if missing := unscoped(lane, scopes); len(missing) > 0 {
		t.Errorf("the %s job runs `%s`, and its scope %v does not claim %s.\n"+
			"A change to only that file skips the job, and the release finds the break. Add the path to the scope in %s.",
			lane.name, releaseLaneMake, lane.scopes, strings.Join(missing, ", "), classifierWorkflow)
	}
}

// releaseJob is as much of a workflow job as the ordering check reads.
type releaseJob struct {
	Needs stringList    `yaml:"needs"`
	Steps []releaseStep `yaml:"steps"`
}

type releaseStep struct {
	Run string `yaml:"run"`
}

// testsFirst reports whether a job runs the lane before any release script, and
// whether it calls one at all.
func testsFirst(job releaseJob) (tested, calls bool) {
	for _, step := range job.Steps {
		lane := strings.Index(step.Run, releaseLaneMake)
		call := releaseScriptCall.FindStringIndex(step.Run)
		switch {
		case call != nil && (lane < 0 || call[0] < lane):
			return false, true
		case lane >= 0:
			return true, true
		}
	}
	return false, false
}

// untestedReleaseJobs names each job that calls a release script too early.
// Neither its own steps nor a job it needs ran the lane before the call.
func untestedReleaseJobs(jobs map[string]releaseJob) (untested []string, calls int) {
	var covered func(name string, seen map[string]bool) bool
	covered = func(name string, seen map[string]bool) bool {
		if seen[name] {
			return false
		}
		seen[name] = true
		if first, _ := testsFirst(jobs[name]); first {
			return true
		}
		for _, need := range jobs[name].Needs {
			if covered(need, seen) {
				return true
			}
		}
		return false
	}
	for name, job := range jobs {
		if _, call := testsFirst(job); !call {
			continue
		}
		calls++
		if !covered(name, map[string]bool{}) {
			untested = append(untested, name)
		}
	}
	slices.Sort(untested)
	return untested, calls
}

func TestEveryReleaseWorkflowTestsItsScriptsBeforeItCallsThem(t *testing.T) {
	t.Parallel()
	calls := 0
	for _, path := range releaseWorkflows(t) {
		body, err := gatekit.ReadFile(filepath.Join("..", path))
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		var parsed struct {
			Jobs map[string]releaseJob `yaml:"jobs"`
		}
		if err := unmarshalYAML(body, &parsed); err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		untested, found := untestedReleaseJobs(parsed.Jobs)
		calls += found
		for _, name := range untested {
			t.Errorf("job %q in %s calls a release script, and neither it nor a job it needs runs `%s` first.\n"+
				"`make check` does not test the release scripts, so a release must. Add the step before the first call, "+
				"or make the job need one that has it.", name, path, releaseLaneMake)
		}
	}
	if calls < minReleaseCalls {
		t.Fatalf("found %d job(s) that call a release script; the release workflows have at least %d", calls, minReleaseCalls)
	}
}

func TestTheOrderingCheckRefusesAJobThatCallsFirst(t *testing.T) {
	t.Parallel()
	steps := func(runs ...string) []releaseStep {
		out := make([]releaseStep, 0, len(runs))
		for _, run := range runs {
			out = append(out, releaseStep{Run: run})
		}
		return out
	}
	jobs := map[string]releaseJob{
		"version":   {Steps: steps(releaseLaneMake, "bash scripts/release-tag-version.sh v1.0.0")},
		"build":     {Needs: stringList{"version"}, Steps: steps("bash scripts/release-bundle.sh")},
		"early":     {Steps: steps("bash scripts/release-tag-version.sh v1.0.0", releaseLaneMake)},
		"orphan":    {Needs: stringList{"early"}, Steps: steps("python3 scripts/release_notes.py")},
		"unrelated": {Steps: steps("make sbom")},
		"sameStep":  {Steps: steps(releaseLaneMake + "\nbash scripts/release-bundle.sh")},
		"callFirst": {Steps: steps("bash scripts/release-bundle.sh\n" + releaseLaneMake)},
	}
	untested, calls := untestedReleaseJobs(jobs)
	if want := []string{"callFirst", "early", "orphan"}; !slices.Equal(untested, want) {
		t.Errorf("untested = %v, want %v", untested, want)
	}
	if calls != 6 {
		t.Errorf("calls = %d, want 6", calls)
	}
}
