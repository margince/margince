// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every change that can make the approval reading catalog stale runs fe-drift.
//
// A Node script builds the catalog from frontend sources, and only a fresh
// build shows that the committed copy is stale. fe-drift does that build, so
// each path that changes the catalog must route a pull request to its lane.

import (
	"slices"
	"strings"
	"testing"
)

const approvalReadingCheck = "gen:approval-reading --check"

func TestEveryChangeToTheApprovalReadingCatalogRunsItsDriftCheck(t *testing.T) {
	t.Parallel()
	scope := filterScopes(t, readWorkflow(t, classifierWorkflow))[theFrontendFilter]
	for _, path := range []string{
		"frontend/scripts/gen-approval-reading.mjs",
		"frontend/src/lib/approvalpreview.ts",
		"frontend/src/i18n/en.ts",
		"backend/internal/modules/agents/previewcatalog_gen.json",
	} {
		if !scopeClaims(path, scope) {
			t.Errorf("the %q filter in %s does not cover %s, so a pull request that changes it "+
				"skips fe-drift and a stale approval reading catalog merges green", theFrontendFilter, classifierWorkflow, path)
		}
	}
	targets, _ := parseRecipes(t, rootMakefile)
	drift, ok := targets["fe-drift"]
	if !ok {
		t.Fatalf("%s has no fe-drift target; it is the lane that rebuilds the approval reading catalog", rootMakefile)
	}
	if !slices.ContainsFunc(drift.recipes, func(line string) bool { return strings.Contains(line, approvalReadingCheck) }) {
		t.Errorf("fe-drift in %s no longer runs %q, so no lane compares the committed approval reading catalog with a fresh build",
			rootMakefile, approvalReadingCheck)
	}
}
