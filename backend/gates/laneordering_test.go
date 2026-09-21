// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

//go:build !integration

package gates

import (
	"path/filepath"
	"regexp"
	"testing"
)

// The craftsmanship gate runs only after the deterministic gates are green: a
// red build must never be judged on style. A structural assertion over the
// tracked workflow, not a live GitHub call.
//
// Named laneordering rather than laneorder, because `make test-laneorder`
// already exists and is about the integration lane's dispatch order — an
// unrelated thing. Two gates whose names differ by one character invite the next
// reader to assume one is the other.
func TestCIWorkflow_craftsmanshipRunsAfterDeterministicGates(t *testing.T) {
	t.Parallel()
	s := readRepoFile(t, filepath.Join(repoRoot, ".github/workflows/ci.yml"))

	for _, job := range []string{"deterministic-gates:", "craftsmanship:"} {
		if !regexp.MustCompile(`(?m)^\s+` + regexp.QuoteMeta(job)).MatchString(s) {
			t.Errorf("ci.yml missing job %q", job)
		}
	}
	// The ordering invariant: craftsmanship declares a dependency on the gate job.
	if !regexp.MustCompile(`needs:\s*\[?\s*deterministic-gates`).MatchString(s) {
		t.Error("craftsmanship job must `needs: [deterministic-gates]` so it runs only after the deterministic gates are green")
	}
}
