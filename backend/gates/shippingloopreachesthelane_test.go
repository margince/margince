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
	integrationScope   = "needs.changes.outputs.backend_db"
	integrationRoutine = "check-all"
	packageRoutine     = "test-it"
)

func TestEveryBackendPullRequestRunsTheIntegrationLane(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(aggregateWorkflow) // #nosec G304 -- a fixed repo-relative path
	if err != nil {
		t.Fatalf("reading %s: %v", aggregateWorkflow, err)
	}
	var wf aggregateJobs
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatalf("%s: %v", aggregateWorkflow, err)
	}
	if !slices.Contains(wf.Jobs[aggregateJob].Needs, integrationJob) {
		t.Errorf("the required %q check no longer needs %q, so a red integration lane merges",
			aggregateJob, integrationJob)
	}
	if !strings.Contains(wf.Jobs[integrationJob].If, integrationScope) {
		t.Errorf("job %q no longer runs on %s, so this gate cannot say which changes reach it",
			integrationJob, integrationScope)
	}
	scope := section(t, string(raw), "backend_db: &backend_db")
	if end := strings.Index(scope, "\n            frontend:"); end >= 0 {
		scope = scope[:end]
	}
	if !strings.Contains(scope, "- 'backend/**'") {
		t.Errorf("the backend_db scope no longer lists 'backend/**', so a backend Go change can " +
			"skip the integration lane")
	}
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
