// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

//go:build !integration

package gates

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What a contributor arriving from outside is promised, in the files they meet
// on the way in. Each assertion is a sentence somebody can delete without
// anything else in the tree noticing.
func TestPRTemplate_requiresTheAccountabilitySections(t *testing.T) {
	t.Parallel()
	tmpl := readRepoFile(t, filepath.Join(repoRoot, ".github/PULL_REQUEST_TEMPLATE.md"))
	for _, want := range []string{"## What", "## Why", "How verified", "AI involvement", "explain every line"} {
		if !strings.Contains(tmpl, want) {
			t.Errorf("PR template missing %q", want)
		}
	}
}

func TestContributing_statesAccountabilityAndDisclosure(t *testing.T) {
	t.Parallel()
	doc := readRepoFile(t, filepath.Join(repoRoot, "CONTRIBUTING.md"))
	for _, want := range []string{"accountable", "explain every line", "craftsmanship gate"} {
		if !strings.Contains(doc, want) {
			t.Errorf("CONTRIBUTING.md missing %q", want)
		}
	}
}

func TestCIWorkflow_externalPRsHitTheGateJobs(t *testing.T) {
	t.Parallel()
	yml := readRepoFile(t, filepath.Join(repoRoot, ".github/workflows/ci.yml"))
	// The craftsmanship + craft-residue jobs are defined so external/fork PRs hit
	// the same gate as internal work.
	for _, job := range []string{"craftsmanship:", "craft-residue:"} {
		if !strings.Contains(yml, job) {
			t.Errorf("ci.yml missing job %q that external PRs must also hit", job)
		}
	}
	// CI is enabled: the automatic pull_request trigger is what makes external PRs
	// hit these jobs without a maintainer, and ready_for_review fires the full suite
	// when a draft PR is marked ready.
	if !regexp.MustCompile(`(?m)^\s+pull_request:`).MatchString(yml) {
		t.Error("ci.yml must trigger on pull_request so external PRs hit the gate jobs")
	}
	if !strings.Contains(yml, "ready_for_review") {
		t.Error("ci.yml pull_request trigger must include ready_for_review")
	}
}
