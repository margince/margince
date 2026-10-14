// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A backend change cannot land without the integration lane. CI's required
// `ci` check runs it on every PR that touches backend/. `make check` cannot:
// every integration file carries `//go:build integration`. Each claim below is
// one link from a backend diff to the lane, read from the file that holds it.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	integrationJob     = "integration"
	integrationScope   = "backend_db"
	integrationRoutine = "check-all"
	packageRoutine     = "test-it"
	scopeJob           = "changes"
	scopeStep          = "filter"
	backendPaths       = "backend/**"
)

type scopedWorkflow struct {
	Jobs map[string]struct {
		Needs []string `yaml:"needs"`
		If    string   `yaml:"if"`
		Steps []struct {
			ID   string         `yaml:"id"`
			With map[string]any `yaml:"with"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func TestEveryBackendPullRequestRunsTheIntegrationLane(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(aggregateWorkflow) // #nosec G304 -- a fixed repo-relative path
	if err != nil {
		t.Fatalf("reading %s: %v", aggregateWorkflow, err)
	}
	var wf scopedWorkflow
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", aggregateWorkflow, err)
	}
	agg, ok := wf.Jobs[aggregateJob]
	if !ok {
		t.Fatalf("%s declares no %q job, so nothing requires the integration lane", aggregateWorkflow, aggregateJob)
	}
	lane, ok := wf.Jobs[integrationJob]
	if !ok {
		t.Fatalf("%s declares no %q job, so no PR runs the integration lane", aggregateWorkflow, integrationJob)
	}
	if !slices.Contains(agg.Needs, integrationJob) {
		t.Errorf("the required %q check no longer needs %q, so a red integration lane merges",
			aggregateJob, integrationJob)
	}
	trigger := "needs." + scopeJob + ".outputs." + integrationScope + " == 'true'"
	if !strings.Contains(normalizeCondition(lane.If), trigger) {
		t.Errorf("job %q no longer runs on `%s`, so this gate cannot say which changes reach it",
			integrationJob, trigger)
	}
	if !slices.Contains(scopeFilters(t, wf)[integrationScope], any(backendPaths)) {
		t.Errorf("the %s scope no longer lists %q, so a backend Go change can skip the integration lane",
			integrationScope, backendPaths)
	}
}

// scopeFilters parses the paths-filter step's `filters` block, so a path is
// judged by the scope that holds it and the YAML anchors resolve as CI's do.
func scopeFilters(t *testing.T, wf scopedWorkflow) map[string][]any {
	t.Helper()
	for _, step := range wf.Jobs[scopeJob].Steps {
		if step.ID != scopeStep {
			continue
		}
		block, ok := step.With["filters"].(string)
		if !ok {
			t.Fatalf("the %s step of job %q carries no `filters` block", scopeStep, scopeJob)
		}
		var filters map[string][]any
		if err := yaml.Unmarshal([]byte(block), &filters); err != nil {
			t.Fatalf("the %s step's filters: %v", scopeStep, err)
		}
		if _, ok := filters[integrationScope]; !ok {
			t.Fatalf("the %s step declares no %q scope", scopeStep, integrationScope)
		}
		return filters
	}
	t.Fatalf("job %q has no step with id %q, so no scope decides which PRs run the lane", scopeJob, scopeStep)
	return nil
}

func TestTheShippingLoopNamesTheIntegrationRoutines(t *testing.T) {
	t.Parallel()
	rules := readRepoFile(t, filepath.Join(repoRoot, "AGENTS.md"))
	shipping := section(t, rules, "## Shipping a change")
	for _, routine := range []string{packageRoutine, integrationRoutine} {
		if !strings.Contains(shipping, "`make "+routine) {
			t.Errorf("the shipping loop does not name `make %s`", routine)
		}
	}

	root := readRepoFile(t, filepath.Join(repoRoot, "Makefile"))
	if !invokes(section(t, root, "\n"+integrationRoutine+":"), "test-integration") {
		t.Errorf("`make %s` no longer runs test-integration, so the rulebook points at a "+
			"routine that reports success without running the lane", integrationRoutine)
	}
}

// invokes reports whether a recipe RUNS a target rather than merely mentioning
// it. The recipe carries the reasoning for what it does and does not run, so
// the lane's name appears in its comments whatever the commands do — and a
// recipe that stopped running the lane would keep every one of those words.
func invokes(recipe, target string) bool {
	for line := range strings.SplitSeq(recipe, "\n") {
		command := strings.TrimSpace(line)
		if strings.HasPrefix(command, "#") {
			continue
		}
		if strings.Contains(command, "$(MAKE)") && strings.Contains(command, target) {
			return true
		}
	}
	return false
}

// section is the text from a heading or a recipe's first line to the start of
// the next one — a markdown "## " heading, or a Makefile recipe's unindented
// target line. Enough to read one step or one recipe whole without parsing
// either format, and it must be WHOLE: a reader that stopped at the first blank
// line would miss a numbered step two paragraphs down and report it missing.
func section(t *testing.T, text, start string) string {
	t.Helper()
	at := strings.Index(text, start)
	if at < 0 {
		t.Fatalf("%q is no longer in the file — this gate is reading a shape that is gone, which "+
			"is how a census comes to certify nothing", start)
	}
	rest := text[at+len(start):]
	for _, boundary := range []string{"\n## ", "\n\n\n"} {
		if end := strings.Index(rest, boundary); end >= 0 {
			rest = rest[:end]
		}
	}
	return rest
}
