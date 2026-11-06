// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// CI splits the backend gate's fan-out over two jobs: deterministic-gates runs
// `make check-backend CHECK_LEGS="…"` and backend-lint runs `make check-static`.
// Either may be a matrix whose entries each pass their share of the legs.
// A leg that neither names still passes `ci`, because nothing ran it. This test
// holds the two jobs to the full CHECK_LEGS list a local `make check` runs.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	backendMakefile = "Makefile"
	testLegsJob     = "deterministic-gates"
	staticLegsJob   = "backend-lint"
)

// makeCommand is one `make <target>` shell command: group 1 the target, group 2
// its arguments up to the end of that command.
var makeCommand = regexp.MustCompile(`\bmake\s+(check-backend|check-static)(?:[ \t]+([^\n;&|]*))?(?:[\n;&|]|$)`)

// commandLineVar is a make variable assigned on the command line, in each
// spelling a shell passes it: double-quoted, single-quoted or bare.
var commandLineVar = regexp.MustCompile(`\b([A-Z_]+)=(?:"([^"]*)"|'([^']*)'|([^\s'"$]*))(?:\s|$)`)

var matrixRef = regexp.MustCompile(`^\$\{\{\s*matrix\.([a-z_-]+)\s*\}\}$`)

type stepRuns struct {
	Jobs map[string]struct {
		Needs    stringList     `yaml:"needs"`
		Steps    []workflowStep `yaml:"steps"`
		Strategy struct {
			Matrix struct {
				Include []map[string]string `yaml:"include"`
			} `yaml:"matrix"`
		} `yaml:"strategy"`
	} `yaml:"jobs"`
}

func TestTheTwoBackendJobsRunEveryCheckLeg(t *testing.T) {
	t.Parallel()
	all, static := backendCheckLegs(t)

	raw, err := gatekit.ReadFile(aggregateWorkflow) // #nosec G304 -- a fixed repo-relative path
	if err != nil {
		t.Fatalf("reading %s: %v", aggregateWorkflow, err)
	}
	var wf stepRuns
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", aggregateWorkflow, err)
	}
	testLegs, ok := legsRunBy(t, wf, testLegsJob, "check-backend", "CHECK_LEGS", all)
	if !ok {
		t.Fatalf("job %q has no `make check-backend` step, so this test cannot tell which legs it runs", testLegsJob)
	}
	staticRan, ok := legsRunBy(t, wf, staticLegsJob, "check-static", "STATIC_LEGS", static)
	if !ok {
		t.Fatalf("job %q has no `make check-static` step, so nothing in CI runs the legs %v", staticLegsJob, static)
	}

	for _, job := range []string{testLegsJob, staticLegsJob} {
		if !slices.Contains(wf.Jobs[aggregateJob].Needs, job) {
			t.Errorf("job %q is not in %q's needs, so the required check passes whatever its legs report", job, aggregateJob)
		}
	}

	ran := append(slices.Clone(testLegs), staticRan...)
	for _, leg := range all {
		if !slices.Contains(ran, leg) {
			t.Errorf("CHECK_LEGS leg %q runs in a local `make check` but in neither %s (CHECK_LEGS=%q) nor %s (%v). "+
				"Add it to one of them, or `ci` passes without it.", leg, testLegsJob, strings.Join(testLegs, " "), staticLegsJob, staticRan)
		}
	}
}

// backendCheckLegs reads CHECK_LEGS and the legs check-static runs, both
// expanded through the Makefile's literal variables.
func backendCheckLegs(t *testing.T) (all, static []string) {
	t.Helper()
	body, err := gatekit.ReadFile(backendMakefile)
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

// legsRunBy answers the legs a job's `make <target>` step runs. With no
// variable assigned, those are the Makefile's own list. Otherwise they are the
// literal value, or a matrix value unioned over every matrix entry.
func legsRunBy(t *testing.T, wf stepRuns, job, target, variable string, defaults []string) ([]string, bool) {
	t.Helper()
	for _, step := range wf.Jobs[job].Steps {
		value, set, found, err := legsOverride(step.Run, target, variable)
		if err != nil {
			t.Fatalf("job %q: %v", job, err)
		}
		if !found {
			continue
		}
		if !set {
			return defaults, true
		}
		return matrixLegs(t, wf, job, value), true
	}
	return nil, false
}

// legsOverride reads the value the `make <target>` command in run assigns to
// variable. A mention it cannot read is an error, never "no override", because
// the default list is every leg and would pass this gate.
func legsOverride(run, target, variable string) (value string, set, found bool, err error) {
	for _, command := range makeCommand.FindAllStringSubmatch(strings.ReplaceAll(run, "\\\n", " "), -1) {
		if command[1] != target {
			continue
		}
		args := command[2]
		for _, assignment := range commandLineVar.FindAllStringSubmatch(args, -1) {
			if assignment[1] == variable {
				return assignment[2] + assignment[3] + assignment[4], true, true, nil
			}
		}
		if strings.Contains(args, variable) {
			return "", false, true, fmt.Errorf("`make %s %s` passes %s in a shape this gate cannot read; "+
				"quote the value, or pass it bare with no $", target, args, variable)
		}
		return "", false, true, nil
	}
	return "", false, false, nil
}

// matrixLegs reads a legs override: literal, or a matrix value unioned over
// every include entry, each of which must set it.
func matrixLegs(t *testing.T, wf stepRuns, job, override string) []string {
	t.Helper()
	key := matrixRef.FindStringSubmatch(override)
	if key == nil {
		return strings.Fields(override)
	}
	include := wf.Jobs[job].Strategy.Matrix.Include
	if len(include) == 0 {
		t.Fatalf("job %q passes its legs from matrix.%s, but has no matrix include entries", job, key[1])
	}
	var ran []string
	for i, entry := range include {
		legs := strings.Fields(entry[key[1]])
		if len(legs) == 0 {
			t.Errorf("job %q matrix include entry %d sets no %s, so that leg runs no check target", job, i, key[1])
		}
		ran = append(ran, legs...)
	}
	return ran
}

func TestLegsOverrideReadsEveryShellSpellingOfTheVariable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, run, value string
		set, found       bool
	}{
		{"double-quoted matrix value", `make check-static STATIC_LEGS="${{ matrix.legs }}"`, "${{ matrix.legs }}", true, true},
		{"single-quoted matrix value", `make check-static STATIC_LEGS='${{ matrix.legs }}'`, "${{ matrix.legs }}", true, true},
		{"single-quoted list", `make check-static STATIC_LEGS='lint-strict vet'`, "lint-strict vet", true, true},
		{"bare leg", `make check-static STATIC_LEGS=lint-strict`, "lint-strict", true, true},
		{"empty double quotes", `make check-static STATIC_LEGS=""`, "", true, true},
		{"empty single quotes", `make check-static STATIC_LEGS=''`, "", true, true},
		{"bare empty", `make check-static STATIC_LEGS=`, "", true, true},
		{"extra whitespace", "make   check-static \t STATIC_LEGS='vet'  ", "vet", true, true},
		{"after another variable", `make check-static GOFLAGS=-v STATIC_LEGS='vet'`, "vet", true, true},
		{"continued line", "make check-static \\\n  STATIC_LEGS='vet' 2>&1 | tee log", "vet", true, true},
		{"no override", `make check-static 2>&1 | tee log`, "", false, true},
		{"override on a later command", "make check-static\necho STATIC_LEGS='vet'", "", false, true},
		{"other target only", `make check-backend STATIC_LEGS='vet'`, "", false, false},
		{"longer target name", `make check-static-extra STATIC_LEGS='vet'`, "", false, false},
	}
	for _, tc := range cases {
		value, set, found, err := legsOverride(tc.run, "check-static", "STATIC_LEGS")
		if err != nil || value != tc.value || set != tc.set || found != tc.found {
			t.Errorf("%s: legsOverride(%q) = (%q, set %v, found %v, %v), want (%q, set %v, found %v, nil)",
				tc.name, tc.run, value, set, found, err, tc.value, tc.set, tc.found)
		}
	}
}

func TestLegsOverrideRefusesAnAssignmentItCannotRead(t *testing.T) {
	t.Parallel()
	for _, run := range []string{
		`make check-static STATIC_LEGS=${{ matrix.legs }}`,
		`make check-static STATIC_LEGS=$LEGS`,
		`make check-static STATIC_LEGS="unclosed`,
	} {
		if _, _, _, err := legsOverride(run, "check-static", "STATIC_LEGS"); err == nil {
			t.Errorf("legsOverride(%q) read an assignment it cannot parse as no override", run)
		}
	}
}
