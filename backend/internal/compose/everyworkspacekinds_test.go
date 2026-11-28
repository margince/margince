// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// The binding middleware leaves a kind unbound only if its worker reads every
// workspace row itself. A worker that does so must be on the list. A listed kind
// whose worker does not would run with no workspace bound.
func TestEveryWorkspaceKindsRunTheEveryWorkspacePass(t *testing.T) {
	t.Parallel()
	for kind := range everyWorkspaceKinds {
		if _, declared := jobs.SpecFor(kind); !declared {
			t.Errorf("%s is listed in everyWorkspaceKinds but api/jobs.yaml does not declare it", kind)
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	callers := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "dispatch.go" {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		callers += strings.Count(string(src), "runEveryWorkspacePass(")
	}
	if callers != len(everyWorkspaceKinds) {
		t.Fatalf("%d workers call runEveryWorkspacePass and %d kinds are listed in everyWorkspaceKinds; the two must match",
			callers, len(everyWorkspaceKinds))
	}
}
