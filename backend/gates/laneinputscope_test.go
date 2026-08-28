// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A path-filtered lane runs on a change to the scripts it EXECUTES.
//
// CI classifies a pull request with dorny/paths-filter and gates each lane on
// one of the resulting scopes. That is a real saving and it has one failure
// mode: a scope that omits a file the lane runs means an edit to that file
// alone skips the only lane that would have executed it. The lane reports
// SKIPPED, the aggregate is content, and the edit ships never once run.
//
// It has already happened. `live-boot` is the README quickstart run literally —
// `make seed-dev` then `make verify-boot` — and the `e2e` scope named
// backend/, frontend/, infra/, extensions/, fixtures/ and composition/ but
// neither `scripts/**` nor the Makefile that reaches them. A change to the dev
// seeder and to the boot proof, which is to say a change to precisely what that
// lane does, classified as touching nothing the lane cares about. The `frontend`
// scope had the same hole around check-contract-frontend-drift.sh.
//
// So the obligation is derived from what the lanes RUN rather than kept as a
// list beside them: every root script a filtered job reaches — directly, or
// through a make target and its prerequisites — must be matched by that job's
// own scope. A script added to a lane's recipe next month arrives here as a
// failure instead of as a lane that quietly stopped covering it.
//
// WHAT THIS CANNOT SEE, and it is the honest limit of reading a recipe: a
// script's own dependencies. verify-boot.sh sourcing a helper elsewhere in the
// tree is invisible here, because following that would mean interpreting shell.
// The census counts what a lane names.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	// The classifier and the lanes it gates live in the merge gate; the
	// Makefile it invokes is the root one, which is where every `make <target>`
	// in a workflow resolves.
	classifierWorkflow = workflowDir + "/ci.yml"
	rootMakefile       = "../Makefile"
	// The step that does the classifying, identified by the action it uses
	// rather than by its name or position.
	pathsFilterAction = "dorny/paths-filter"
	// The path every filtered job needs the moment it says `make` at all: the
	// Makefile is what decides which commands the lane runs.
	makefilePath = "Makefile"
)

// A job's guard names the scopes it runs under, e.g.
// `needs.changes.outputs.e2e == 'true' && …`.
var scopeGuard = regexp.MustCompile(`needs\.changes\.outputs\.(\w+)`)

// A repository script as a command names it: `./scripts/x.sh`,
// `bash scripts/x.sh`, `node scripts/x.mjs`, `frontend/scripts/x.sh`.
var scriptReference = regexp.MustCompile(`(?:frontend/)?scripts/[\w.-]+\.(?:sh|mjs|js|ts)`)

// `make check-fe`, `make db-up && make migrate`.
var makeInvocation = regexp.MustCompile(`\bmake\s+([a-z][a-z0-9-]*)`)

// A `NAME=value` word set for the duration of one recipe command.
var environmentAssignment = regexp.MustCompile(`^[A-Z][A-Z0-9_]*=\S*$`)

// A variable reference in either spelling make accepts.
var makeReference = regexp.MustCompile(`\$[({]([A-Z][A-Z0-9_]*)[)}]`)

// workflowFile is as much of a workflow as this gate reads.
type workflowFile struct {
	Jobs map[string]struct {
		If    string `yaml:"if"`
		Uses  string `yaml:"uses"`
		Steps []struct {
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
			With struct {
				Filters string `yaml:"filters"`
			} `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readWorkflow(t *testing.T, path string) workflowFile {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var parsed workflowFile
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return parsed
}

// filterScopes is the classifier's own declaration: scope name to the patterns
// a path is matched against. The `filters:` input is YAML inside YAML, and its
// anchors mean one scope can be spelled as another plus extras — which is why
// the value is flattened rather than read as a flat list.
func filterScopes(t *testing.T, workflow workflowFile) map[string][]string {
	t.Helper()
	for _, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if !strings.Contains(step.Uses, pathsFilterAction) {
				continue
			}
			var declared map[string]yaml.Node
			if err := yaml.Unmarshal([]byte(step.With.Filters), &declared); err != nil {
				t.Fatalf("parsing the %s filters: %v", pathsFilterAction, err)
			}
			scopes := map[string][]string{}
			for name, patterns := range declared {
				scopes[name] = flattenPatterns(&patterns)
			}
			return scopes
		}
	}
	t.Fatalf("no step in %s uses %s — the classifier was renamed or removed, and this gate was about to compare nothing", classifierWorkflow, pathsFilterAction)
	return nil
}

// flattenPatterns reads a scope's value: a list of patterns, and where one scope
// is spelled as another plus extras, an ANCHOR standing for that other list.
// Read as nodes rather than as decoded values so the alias is followed — the
// `backend` scope is `*backend_db` plus the rulebooks, and a reader that stopped
// at the alias would judge it against two paths.
func flattenPatterns(node *yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		return []string{node.Value}
	case yaml.AliasNode:
		return flattenPatterns(node.Alias)
	case yaml.SequenceNode:
		var out []string
		for _, item := range node.Content {
			out = append(out, flattenPatterns(item)...)
		}
		return out
	default:
		// A shape the classifier does not use today. Returning nothing leaves
		// the scope narrower than it is, which fails loudly against a lane
		// rather than quietly claiming a path.
		return nil
	}
}

// recipeTarget is one Makefile rule as this gate reads it: what it pulls in and
// the command lines it runs. parseMakefile next door keeps only WHETHER a target
// works, because that is all a leg census needs; this one needs the commands
// themselves, which is the whole question here.
type recipeTarget struct {
	pulls   []string
	recipes []string
}

func parseRecipes(t *testing.T, path string) (map[string]*recipeTarget, map[string]string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	vars := readLiteralVars(string(body))
	targets := map[string]*recipeTarget{}
	var current []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "\t") {
			for _, name := range current {
				targets[name].recipes = append(targets[name].recipes, line)
			}
			continue
		}
		match := makeRuleLine.FindStringSubmatch(line)
		if strings.HasPrefix(line, "#") || match == nil {
			continue
		}
		current = strings.Fields(match[1])
		for _, name := range current {
			if targets[name] == nil {
				targets[name] = &recipeTarget{}
			}
			targets[name].pulls = append(targets[name].pulls,
				strings.Fields(expandRefs(match[2], vars))...)
		}
	}
	return targets, vars
}

// expandRefs substitutes every `$(NAME)` this Makefile assigns literally, and
// leaves the rest STANDING. Blanking an unknown reference is what make itself
// does, and it is the wrong answer here: `$(MAKE)` is set by make rather than by
// the file, so expanding it to nothing turned every `$(MAKE) fe-drift` line into
// a bare word and the walk stopped following delegations — a census reading a
// smaller graph and reporting a pass, which is the one way it must not break.
//
// The honest limit is a script named through a variable built from another
// variable: the ceiling is a few passes, and the top of this file says so.
func expandRefs(text string, vars map[string]string) string {
	for range 5 {
		next := makeReference.ReplaceAllStringFunc(text, func(ref string) string {
			name := makeVariable.FindStringSubmatch(ref)
			if value, ok := vars[nameOf(name)]; ok {
				return value
			}
			return ref
		})
		if next == text {
			return text
		}
		text = next
	}
	return text
}

// scriptsUnder walks a make target over its prerequisites and its sub-make
// delegations, collecting every repository script the recipes name.
func scriptsUnder(targets map[string]*recipeTarget, vars map[string]string, root string) []string {
	found := map[string]bool{}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		target := targets[name]
		if target == nil {
			return
		}
		next := slices.Clone(target.pulls)
		for _, recipe := range target.recipes {
			command := expandRefs(recipe, vars)
			for _, script := range scriptReference.FindAllString(command, -1) {
				found[script] = true
			}
			next = append(next, delegatedTargets(command)...)
		}
		for _, name := range next {
			walk(name)
		}
	}
	walk(root)
	return sorted(found)
}

// delegatedTargets reads the targets a `$(MAKE) a b` line pulls in. A `-C` line
// is a sub-make into another directory: it runs a different Makefile, so it adds
// no edge to this graph and its scripts are that directory's own.
//
// The environment prefix is stripped first, because `make check` is spelled
// `@PHASE_TIMER_OWNED=1 $(MAKE) check-backend` — a reader that required the
// line to OPEN with $(MAKE) followed neither half of the merge gate and judged
// it against one timer script.
func delegatedTargets(command string) []string {
	body, isSubMake := strings.CutPrefix(
		stripEnvironment(strings.TrimLeft(strings.TrimPrefix(command, "\t"), " \t@+-")), "$(MAKE)")
	if !isSubMake || strings.Contains(body, " -C ") {
		return nil
	}
	var targets []string
	for _, token := range strings.Fields(body) {
		if strings.HasPrefix(token, "-") || strings.ContainsAny(token, "$()") {
			continue
		}
		targets = append(targets, token)
	}
	return targets
}

// stripEnvironment drops the `NAME=value ` words a recipe sets for the command
// that follows, which is where a recursive make often begins.
func stripEnvironment(body string) string {
	for {
		first, rest, found := strings.Cut(body, " ")
		if !found || !environmentAssignment.MatchString(first) {
			return body
		}
		body = strings.TrimLeft(rest, " \t")
	}
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// filteredLane is one CI job that runs only when the classifier says its scope
// changed, together with every repository input its steps name.
type filteredLane struct {
	name   string
	scopes []string
	inputs []string
}

// filteredLanes reads the merge gate for jobs guarded by a classifier scope. A
// job that delegates to a reusable lane workflow carries no steps of its own, so
// the callee's steps are read as the job's.
func filteredLanes(t *testing.T, workflow workflowFile) []filteredLane {
	t.Helper()
	targets, vars := parseRecipes(t, rootMakefile)
	var lanes []filteredLane
	for name, job := range workflow.Jobs {
		guarded := scopeGuard.FindAllStringSubmatch(job.If, -1)
		if guarded == nil {
			continue
		}
		lane := filteredLane{name: name}
		for _, match := range guarded {
			lane.scopes = append(lane.scopes, match[1])
		}
		commands := jobCommands(t, workflow, job.Uses, name)
		inputs := map[string]bool{}
		for _, command := range commands {
			for _, script := range scriptReference.FindAllString(command, -1) {
				inputs[script] = true
			}
			for _, invocation := range makeInvocation.FindAllStringSubmatch(command, -1) {
				inputs[makefilePath] = true
				for _, script := range scriptsUnder(targets, vars, invocation[1]) {
					inputs[script] = true
				}
			}
		}
		lane.inputs = sorted(inputs)
		lanes = append(lanes, lane)
	}
	slices.SortFunc(lanes, func(a, b filteredLane) int { return strings.Compare(a.name, b.name) })
	return lanes
}

// jobCommands is every `run:` in the job, following a `uses:` into the reusable
// lane workflow that holds the steps.
func jobCommands(t *testing.T, workflow workflowFile, uses, jobName string) []string {
	t.Helper()
	var commands []string
	for _, step := range workflow.Jobs[jobName].Steps {
		if step.Run != "" {
			commands = append(commands, step.Run)
		}
	}
	if !strings.HasPrefix(uses, "./") {
		return commands
	}
	lane := readWorkflow(t, filepath.Join("..", strings.TrimPrefix(uses, "./")))
	for _, job := range lane.Jobs {
		for _, step := range job.Steps {
			if step.Run != "" {
				commands = append(commands, step.Run)
			}
		}
	}
	return commands
}

// unscoped answers which of a lane's inputs no scope of its own claims.
func unscoped(lane filteredLane, scopes map[string][]string) []string {
	var missing []string
	for _, input := range lane.inputs {
		claimed := false
		for _, scope := range lane.scopes {
			claimed = claimed || scopeClaims(input, scopes[scope])
		}
		if !claimed {
			missing = append(missing, input)
		}
	}
	return missing
}

// scopeClaims answers the three pattern shapes the classifier uses: a literal
// path, a directory tree, and a basename anywhere in the tree. A shape it does
// not recognise leaves the path unclaimed rather than claimed, because the way
// this gate must not fail is by reporting a pass over patterns it never read.
func scopeClaims(path string, patterns []string) bool {
	for _, pattern := range patterns {
		pattern = strings.Trim(pattern, "'")
		switch {
		case pattern == path:
			return true
		case strings.HasSuffix(pattern, "/**") &&
			strings.HasPrefix(path, strings.TrimSuffix(pattern, "**")):
			return true
		case strings.HasPrefix(pattern, "**/") &&
			(path == strings.TrimPrefix(pattern, "**/") ||
				strings.HasSuffix(path, strings.TrimPrefix(pattern, "**"))):
			return true
		}
	}
	return false
}

func TestEveryFilteredLaneRunsOnAChangeToTheScriptsItExecutes(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, classifierWorkflow)
	scopes := filterScopes(t, workflow)
	lanes := filteredLanes(t, workflow)

	// A classifier that stopped being recognised, or a workflow whose jobs
	// stopped being guarded, would leave nothing to judge and report a pass.
	if len(lanes) == 0 {
		t.Fatalf("%s has no job guarded by a classifier scope — this census read an empty tree", classifierWorkflow)
	}
	if len(scopes) == 0 {
		t.Fatalf("the %s step declares no scopes — this census had nothing to match against", pathsFilterAction)
	}
	for _, lane := range lanes {
		missing := unscoped(lane, scopes)
		if len(missing) == 0 {
			continue
		}
		t.Errorf("the %s job runs %s, and its scope %v claims none of them.\n"+
			"An edit to one of those files alone classifies as touching nothing this lane cares about: "+
			"the lane reports SKIPPED, the aggregate is content, and the change ships never once executed.\n"+
			"Add the paths to the scope in %s.",
			lane.name, strings.Join(missing, ", "), lane.scopes, classifierWorkflow)
	}
}

func TestAScriptReachedOnlyThroughMakeIsStillALaneInput(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, classifierWorkflow)
	lanes := filteredLanes(t, workflow)

	// The defect this gate was written for was reached through `make`, not named
	// in the workflow: live-boot's steps say `make seed-dev` and the seeder's
	// path appears nowhere in ci.yml. A walk that stopped at the make target
	// would judge that lane against the Makefile alone and pass it, so at least
	// one lane must owe a script no step spells out.
	reached := map[string][]string{}
	for _, lane := range lanes {
		named := strings.Join(jobCommands(t, workflow, workflow.Jobs[lane.name].Uses, lane.name), "\n")
		for _, input := range lane.inputs {
			if input != makefilePath && !strings.Contains(named, input) {
				reached[lane.name] = append(reached[lane.name], input)
			}
		}
	}
	if len(reached) > 0 {
		return
	}
	targets, vars := parseRecipes(t, rootMakefile)
	t.Errorf("no lane owes a script that only its make targets name, and one does: `make seed-dev` reaches %s.\n"+
		"The recipe walk stopped following prerequisites and $(MAKE) delegations, which leaves every lane judged against the Makefile alone.",
		strings.Join(scriptsUnder(targets, vars, "seed-dev"), ", "))
}

func TestTheCensusFailsOnAScopeThatOmitsAScriptItsLaneRuns(t *testing.T) {
	t.Parallel()
	workflow := readWorkflow(t, classifierWorkflow)
	scopes := filterScopes(t, workflow)
	lanes := filteredLanes(t, workflow)

	// The gate's own falsification, and the defect it shipped against: take the
	// live classifier, drop the scripts from one scope, and the lane that runs
	// them must come back unclaimed. Without this, a matcher that quietly
	// claimed everything would keep this file green forever.
	for _, lane := range lanes {
		narrowed := map[string][]string{}
		for name, patterns := range scopes {
			kept := make([]string, 0, len(patterns))
			for _, pattern := range patterns {
				if !slices.Contains(lane.scopes, name) || !strings.HasPrefix(pattern, "scripts/") {
					kept = append(kept, pattern)
				}
			}
			narrowed[name] = kept
		}
		scripts := 0
		for _, input := range lane.inputs {
			if strings.HasPrefix(input, "scripts/") {
				scripts++
			}
		}
		if scripts == 0 {
			continue
		}
		if missing := unscoped(lane, narrowed); len(missing) == 0 {
			t.Errorf("%s: %d root script(s) this lane runs stayed claimed after every 'scripts/…' pattern was removed from its scope %v — the matcher claims paths no pattern covers, so the census cannot fail",
				lane.name, scripts, lane.scopes)
		}
	}
}

func TestARecursiveMakeBehindAnEnvironmentPrefixIsStillAnEdge(t *testing.T) {
	t.Parallel()

	// `make check` is spelled `@PHASE_TIMER_OWNED=1 $(MAKE) check-backend`, and a
	// reader that required the line to open with $(MAKE) followed neither half of
	// the merge gate — it saw one timer script where two whole trees of gates
	// hang. The census shape that fails short is the one this file exists to
	// refuse, so the spelling is a case rather than a comment.
	for command, want := range map[string][]string{
		"\t@PHASE_TIMER_OWNED=1 $(MAKE) check-backend":   {"check-backend"},
		"\tPHASE_TIMER_OWNED=1 $(MAKE) -C backend check": nil,
		"\t$(MAKE) fe-drift":                             {"fe-drift"},
		"\t@bash scripts/phase-timer.sh reset":           nil,
	} {
		if got := delegatedTargets(command); !slices.Equal(got, want) {
			t.Errorf("delegatedTargets(%q) = %v, want %v", command, got, want)
		}
	}
}

func TestTheRecipeWalkReadsPrerequisitesAndDelegations(t *testing.T) {
	t.Parallel()
	targets, vars := parseRecipes(t, rootMakefile)

	// Two edges, both load-bearing, both invisible from a target's own recipe:
	// `make check` reaches its script gates through prerequisites, and the
	// frontend legs are pulled in by $(MAKE) lines. A walk that read only the
	// named target's own commands would find nothing behind either.
	for _, root := range []string{"check", "check-fe"} {
		if _, ok := targets[root]; !ok {
			t.Fatalf("%s declares no %q target — the walk was about to prove nothing", rootMakefile, root)
		}
		if scripts := scriptsUnder(targets, vars, root); len(scripts) == 0 {
			t.Errorf("the walk found no script behind `make %s`, which runs several: %s",
				root, fmt.Sprint(targets[root].pulls))
		}
	}
}
