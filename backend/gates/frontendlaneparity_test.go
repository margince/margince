// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The frontend gate is spelled once as `make check-fe` and run by CI as three
// parallel jobs. That split is a standing invitation to drift: a leg added to
// the local gate that no CI job invokes runs on developer machines and never on
// a pull request, which is a gate that silently checks nothing — and the
// direction with teeth, because the local run is the one somebody skips.
//
// So the obligation is derived from both artifacts rather than listed. A LEG is
// any target whose recipe does work of its own, as opposed to a target that only
// groups others; every leg the local gate reaches must also be reached by a job
// the required `frontend` check depends on. Both halves matter, and both are
// mistakes this gate has already had to be taught:
//
//   - Work is found by RECIPE, not by target name. Until this lane was split,
//     `frontend-check` was a list of raw script lines, so "add my new check to
//     frontend-check" is the next edit a reasonable engineer makes — and a model
//     that only followed `$(MAKE)` edges could not see it at all.
//   - CI coverage is read from the JOB GRAPH, not from the file's text. A
//     `run: make …` line inside a commented-out step, or inside a job the
//     `frontend` fan-in does not depend on, is not a gate that runs.

const (
	// The local gate, and the root of the obligation. `check-fe` rather than
	// `frontend-check` on purpose: it is what `make check` runs, and it adds the
	// composed typecheck and the unit screens' suites, which are gate legs too.
	localGateRoot = "check-fe"
	// The fan-in whose name is the required status check. Only jobs it depends
	// on can block a merge, so only those count as CI coverage.
	requiredFanInJob = "frontend"
)

// makeTarget is one Makefile rule: what it pulls in, and whether it does
// anything itself.
type makeTarget struct {
	pulls []string // prerequisites plus explicit sub-make delegations
	works bool     // has a recipe line that is not purely a local delegation
}

// A rule line: `name: prereq prereq`. Assignments (`X ?= y`, `X := y`) do not
// match, because ?/: are excluded from the name.
var makeRuleLine = regexp.MustCompile(`^([a-z][a-z0-9-]*(?: [a-z][a-z0-9-]*)*):(?:[ \t]+(.*))?$`)

// A literal assignment: `NAME := words`, `NAME = words`, `NAME ?= words`,
// `NAME += words`, optionally continued with a trailing backslash. Only the
// upper-case spelling, which is what this Makefile uses for its own variables.
//
// The operator is captured, because the three do not mean the same thing and
// reading them all as "set" is how a leg goes missing: `LEGS := a` followed by
// `LEGS += b` is `a b` to make, and `b` to a reader that only assigns.
var makeAssignment = regexp.MustCompile(`^([A-Z][A-Z0-9_]*)\s*([:?+]?=)\s*(.*)$`)

// makeVariable expands a `$(NAME)` or `${NAME}` reference.
var makeVariable = regexp.MustCompile(`^\$[({]([A-Z][A-Z0-9_]*)[)}]$`)

// readLiteralVars collects the variables whose value is plain text, so a recipe
// naming one can still be read for the targets it hides.
//
// A value that itself contains `$(` is NOT collected: expanding it would mean
// implementing make, and a half-expansion that produced a plausible word is the
// failure this gate exists to prevent. Such a reference reaches parseRecipe
// unresolved and stops the run there, which is the loud outcome.
func readLiteralVars(body string) map[string]string {
	vars := map[string]string{}
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		match := makeAssignment.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		name, operator, value := match[1], match[2], match[3]
		for strings.HasSuffix(value, `\`) && i+1 < len(lines) {
			i++
			value = strings.TrimSuffix(value, `\`) + " " + strings.TrimSpace(lines[i])
		}
		// make drops a `#` comment from a variable's value; keeping it would
		// expand the comment's words as though they were target names.
		if hash := strings.Index(value, "#"); hash >= 0 {
			value = value[:hash]
		}
		value = strings.TrimSpace(value)
		if strings.Contains(value, "$(") || strings.Contains(value, "${") {
			// Not literal. Drop any earlier literal value with it: a name whose
			// current value this cannot compute must reach parseRecipe
			// unresolved, and a stale earlier reading would resolve it wrongly.
			delete(vars, name)
			continue
		}
		switch operator {
		case "+=":
			// Appends to whatever is there. A name appended to before it was
			// ever set is empty plus the value, which is the value.
			if previous, ok := vars[name]; ok {
				value = previous + " " + value
			}
		case "?=":
			// Assigns only if the name has no value yet, so the FIRST one wins
			// where the others' last one does.
			if _, ok := vars[name]; ok {
				continue
			}
		}
		vars[name] = value
	}
	return vars
}

// parseRecipe reads one recipe line, returning the local targets it delegates
// to and whether the line counts as work of its own.
func parseRecipe(t *testing.T, line string, vars map[string]string) (targets []string, works bool) {
	t.Helper()
	body := strings.TrimLeft(strings.TrimPrefix(line, "\t"), " \t@+-")
	rest, isSubMake := strings.CutPrefix(body, "$(MAKE)")
	if !isSubMake {
		return nil, true // an ordinary command: this target does work
	}
	for _, token := range expandTokens(strings.Fields(rest), vars) {
		switch {
		case token == "-C":
			// A sub-make into another directory (backend/). It does real work
			// and adds no edge inside THIS Makefile's graph.
			return nil, true
		case strings.HasPrefix(token, "-"):
			continue // a flag such as --no-print-directory, or -j4
		case strings.ContainsAny(token, "$()"):
			t.Fatalf("unrecognised sub-make %q — this gate reads $(MAKE) lines to find the legs, so a spelling it cannot parse would silently drop one. A variable holding the legs is fine as long as its value is literal; one built from another variable is not.", strings.TrimSpace(line))
		default:
			targets = append(targets, token)
		}
	}
	return targets, len(targets) == 0
}

// expandTokens replaces each whole-token variable reference with the words it
// stands for. Whole-token only: `-j$(GATE_JOBS)` is already a flag and is left
// for the flag arm, and a variable spliced into the middle of a target name is
// not a spelling this Makefile uses.
func expandTokens(tokens []string, vars map[string]string) []string {
	expanded := make([]string, 0, len(tokens))
	for _, token := range tokens {
		name := makeVariable.FindStringSubmatch(token)
		if value, ok := vars[nameOf(name)]; ok {
			expanded = append(expanded, strings.Fields(value)...)
			continue
		}
		expanded = append(expanded, token)
	}
	return expanded
}

// nameOf is the captured variable name, or "" when the token was not a bare
// variable reference — "" is never a collected name, so the lookup misses.
func nameOf(match []string) string {
	if match == nil {
		return ""
	}
	return match[1]
}

// parseMakefile maps every target to what it pulls in and whether it works.
func parseMakefile(t *testing.T, path string) map[string]*makeTarget {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	targets := map[string]*makeTarget{}
	vars := readLiteralVars(string(body))
	var current []string
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "\t") {
			delegated, works := parseRecipe(t, line, vars)
			for _, name := range current {
				targets[name].pulls = append(targets[name].pulls, delegated...)
				targets[name].works = targets[name].works || works
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
				targets[name] = &makeTarget{}
			}
			targets[name].pulls = append(targets[name].pulls, strings.Fields(match[2])...)
		}
	}
	if targets[localGateRoot] == nil {
		t.Fatalf("%s declares no %q target — the local gate was renamed, so this gate was about to compare nothing", path, localGateRoot)
	}
	return targets
}

// reachable walks from roots over prerequisites and delegations alike.
func reachable(targets map[string]*makeTarget, roots ...string) map[string]bool {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		if target := targets[name]; target != nil {
			for _, next := range target.pulls {
				walk(next)
			}
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return seen
}

// stringList accepts both YAML spellings of `needs:` — one job, or a list.
type stringList []string

func (s *stringList) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var one string
		if err := node.Decode(&one); err != nil {
			return err
		}
		*s = stringList{one}
		return nil
	}
	var many []string
	if err := node.Decode(&many); err != nil {
		return err
	}
	*s = many
	return nil
}

type workflow struct {
	Jobs map[string]struct {
		Needs stringList `yaml:"needs"`
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// ciGateTargets is every make target invoked by a job the required fan-in
// depends on. Parsed as YAML rather than grepped, so a commented-out step or a
// job outside the fan-in's `needs` cannot pass for coverage.
// gateWorkflowDeclaring finds the merge-gate workflow that declares `job`.
//
// The fan-in used to live in ci.yml; it now lives in the frontend lane that
// ci.yml calls. Searching the workflow directory for whichever file declares the
// job means this gate follows the next move too, instead of failing with
// "the lane's work moved out from behind the required check" when the work is
// exactly where it should be — one file over.
func gateWorkflowDeclaring(t *testing.T, job string) string {
	t.Helper()
	paths, err := filepath.Glob("../.github/workflows/*.yml")
	if err != nil {
		t.Fatalf("listing workflows: %v", err)
	}
	var found []string
	for _, path := range paths {
		body, err := os.ReadFile(path) // #nosec G304 -- a repo-relative path from the glob above
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		var parsed workflow
		if err := yaml.Unmarshal(body, &parsed); err != nil {
			// A workflow this gate cannot parse is not necessarily the one it
			// wants; the assertion below fails if none of them is.
			continue
		}
		// The fan-in is the job that DOES the asserting, so it has steps. The
		// caller carries a job of the same name that only delegates (`uses:`),
		// with no steps of its own — matching that one would walk the caller's
		// `needs` (the classifier) instead of the lane's three legs.
		declared, ok := parsed.Jobs[job]
		if ok && len(declared.Needs) > 0 && len(declared.Steps) > 0 {
			found = append(found, path)
		}
	}
	switch len(found) {
	case 1:
		return found[0]
	case 0:
		t.Fatalf("no workflow under ../.github/workflows declares a %q job with needs — the required check was renamed or retired, so nothing here gates the lane", job)
	default:
		t.Fatalf("%d workflows declare a %q fan-in (%s) — two definitions of one required check drift, and this gate cannot tell which one runs", len(found), job, strings.Join(found, ", "))
	}
	return ""
}

func ciGateTargets(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path) // #nosec G304 -- a repo-relative workflow path
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var parsed workflow
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	fanIn, ok := parsed.Jobs[requiredFanInJob]
	if !ok {
		t.Fatalf("%s declares no %q job — the required check was renamed, so nothing here gates the lane", path, requiredFanInJob)
	}
	invoked := regexp.MustCompile(`(?m)^\s*make\s+([a-z][a-z0-9-]*)`)
	var targets []string
	for _, dependency := range fanIn.Needs {
		for _, step := range parsed.Jobs[dependency].Steps {
			for _, match := range invoked.FindAllStringSubmatch(step.Run, -1) {
				targets = append(targets, match[1])
			}
		}
	}
	if len(targets) == 0 {
		t.Fatalf("no job the %q fan-in needs runs a make target in %s — the lane's work moved out from behind the required check", requiredFanInJob, path)
	}
	return targets
}

func TestEveryLocalFrontendGateLegRunsInCI(t *testing.T) {
	targets := parseMakefile(t, "../Makefile")
	local := reachable(targets, localGateRoot)
	if len(local) < 2 {
		t.Fatalf("%q reaches no other target — the local gate lost its legs, or this gate stopped being able to see them", localGateRoot)
	}
	ciRoots := ciGateTargets(t, gateWorkflowDeclaring(t, requiredFanInJob))
	inCI := reachable(targets, ciRoots...)

	for leg := range local {
		// The root is the gate, not a leg of it: CI runs the jobs below it.
		if leg == localGateRoot || !targets[leg].works || inCI[leg] {
			continue
		}
		t.Errorf("`make %s` runs %q but no job behind the %q check reaches it — the leg would run locally and never on a pull request; add it to one of: %s",
			localGateRoot, leg, requiredFanInJob, strings.Join(ciRoots, ", "))
	}
}

// TestASubMakeBehindAVariableIsStillReadAsLegs holds the expansion this gate's
// Makefile reading depends on.
//
// check-backend hands its 30 gates to a sub-make through ROOT_SCRIPT_GATES
// rather than spelling them on the line. Before the expansion existed that
// token stopped the run; the danger in adding it is the opposite one — an
// expansion that quietly produced NOTHING would leave the fan-out looking like
// a sub-make with no legs, which reads exactly like a lane that has none. So
// each arm below states what the reading must be, including the two that must
// still refuse.
func TestASubMakeBehindAVariableIsStillReadAsLegs(t *testing.T) {
	vars := readLiteralVars("LEGS := fe-lint fe-test\nWIDTH := 4\n" +
		"DERIVED := $(LEGS) fe-build\nCONTINUED := fe-a \\\n  fe-b\n")

	for _, c := range []struct {
		name    string
		line    string
		targets []string
		works   bool
	}{
		{"a variable holding the legs expands to them", "\t$(MAKE) $(LEGS)", []string{"fe-lint", "fe-test"}, false},
		{"a continued assignment keeps every leg", "\t$(MAKE) $(CONTINUED)", []string{"fe-a", "fe-b"}, false},
		{"a flag built from a variable stays a flag", "\t$(MAKE) -j$(WIDTH) $(LEGS)", []string{"fe-lint", "fe-test"}, false},
		{"a literal leg beside a variable one is kept", "\t$(MAKE) fe-drift $(LEGS)", []string{"fe-drift", "fe-lint", "fe-test"}, false},
		{"an unknown variable is not silently dropped", "\t$(MAKE) $(NOT_DECLARED)", nil, false},
		{"a sub-make into another directory is work, not an edge", "\t$(MAKE) -C backend check", nil, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.name == "an unknown variable is not silently dropped" {
				// The refusal is a t.Fatalf, so it cannot be called here
				// without ending this test; assert the classification that
				// leads to it instead.
				token := expandTokens([]string{"$(NOT_DECLARED)"}, vars)
				if len(token) != 1 || token[0] != "$(NOT_DECLARED)" {
					t.Fatalf("expandTokens dropped or altered an undeclared variable: %q — it must reach "+
						"parseRecipe intact so the run stops there", token)
				}
				return
			}
			targets, works := parseRecipe(t, c.line, vars)
			if !slices.Equal(targets, c.targets) {
				t.Errorf("parseRecipe(%q) legs = %q, want %q", c.line, targets, c.targets)
			}
			if works != c.works {
				t.Errorf("parseRecipe(%q) works = %v, want %v", c.line, works, c.works)
			}
		})
	}

	// A value built from another variable is NOT collected: half-expanding it
	// would invent a leg list nobody wrote.
	if value, ok := vars["DERIVED"]; ok {
		t.Errorf("DERIVED was collected as %q, but its value names another variable — "+
			"collecting it would make this gate a partial implementation of make", value)
	}
}

// TestTheThreeAssignmentsAreReadAsMakeReadsThem pins the operators apart.
//
// Reading all three as "set" loses legs in the direction that stays green: a
// list built up with `+=` reads as only its last line, and the gate then holds
// a lane to a fraction of what CI runs. Each case below is what `make` itself
// would expand the name to.
func TestTheThreeAssignmentsAreReadAsMakeReadsThem(t *testing.T) {
	for _, c := range []struct {
		name, makefile, want string
	}{
		{"append builds the list up", "LEGS := fe-lint\nLEGS += fe-test\n", "fe-lint fe-test"},
		{"append to an unset name is just the value", "LEGS += fe-test\n", "fe-test"},
		{"conditional assignment keeps the first", "LEGS ?= fe-lint\nLEGS ?= fe-test\n", "fe-lint"},
		{"conditional after a plain one does not replace it", "LEGS := fe-lint\nLEGS ?= fe-test\n", "fe-lint"},
		{"plain reassignment keeps the last", "LEGS := fe-lint\nLEGS := fe-test\n", "fe-test"},
		{"a later derived value withdraws the name", "LEGS := fe-lint\nLEGS := $(OTHER)\n", ""},
		{"appending a derived value withdraws it too", "LEGS := fe-lint\nLEGS += $(OTHER)\n", ""},
		{"a comment tail is not a leg", "LEGS := fe-lint fe-test # and why\n", "fe-lint fe-test"},
		{"a comment tail on an appended line either", "LEGS := fe-lint\nLEGS += fe-test # and why\n", "fe-lint fe-test"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := readLiteralVars(c.makefile)["LEGS"]; got != c.want {
				t.Errorf("readLiteralVars(%q)[LEGS] = %q, want %q", c.makefile, got, c.want)
			}
		})
	}
}

// TestTheFanOutsOwnLegsAreVisibleToThisGate is the live half: the expansion
// above is only worth having if it reaches the real Makefile.
//
// The expectation is DERIVED from the variable the fan-out actually hands over,
// not from a number written here — a pinned count is a second copy of the gate
// list, and it goes stale in the direction that reads green. What is pinned is
// a FLOOR far below the real count, which catches the read itself having broken:
// an empty ROOT_SCRIPT_GATES would otherwise make the loop below assert nothing
// and pass, which is indistinguishable from a fan-out that reaches everything.
func TestTheFanOutsOwnLegsAreVisibleToThisGate(t *testing.T) {
	const fanOutFloor = 20

	body, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("reading ../Makefile: %v", err)
	}
	declared := strings.Fields(readLiteralVars(string(body))["ROOT_SCRIPT_GATES"])
	if len(declared) < fanOutFloor {
		t.Fatalf("ROOT_SCRIPT_GATES reads as %d gate(s), below the floor of %d — the variable was "+
			"renamed, or readLiteralVars stopped collecting it. Either way this gate was about to "+
			"judge the fan-out against an empty list and pass", len(declared), fanOutFloor)
	}

	targets := parseMakefile(t, "../Makefile")
	backend, ok := targets["check-backend"]
	if !ok {
		t.Fatalf("../Makefile declares no check-backend target — this gate was about to read nothing")
	}
	if !slices.Contains(backend.pulls, declared[0]) {
		t.Fatalf("check-backend's fan-out reaches none of ROOT_SCRIPT_GATES — the recipe stopped " +
			"handing the variable over, or stopped being a $(MAKE) line this gate follows")
	}
	// Each expanded word must be a rule this Makefile actually declares. Asking
	// whether the fan-out CONTAINS the names it was handed would prove nothing —
	// both sides are the same expansion, so it holds however wrong the expansion
	// is. Asking whether those names exist is a second, independent reading of
	// the file: an expansion that invented a word answers no.
	for _, gate := range declared {
		if targets[gate] == nil {
			t.Errorf("ROOT_SCRIPT_GATES names %q, which this Makefile declares no rule for — either "+
				"the gate was renamed and the fan-out still asks for the old name, or the variable "+
				"was read wrongly and these are not target names at all", gate)
		}
	}
}
