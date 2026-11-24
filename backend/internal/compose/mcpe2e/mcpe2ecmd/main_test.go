// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// repoRoot is the repository this command reads, from this package's folder.
const repoRoot = "../../../../.."

func declareTree(t *testing.T) {
	t.Helper()
	// Go's test cache never rechecks a file outside this module.
	if err := gatekit.DeclareInputs(os.Getenv, repoRoot+"/docs", repoRoot+"/e2e"); err != nil {
		t.Fatal(err)
	}
}

func TestTheReportCountsEveryCommittedVerdictFolder(t *testing.T) {
	declareTree(t)
	var out strings.Builder
	if err := run([]string{"report", "-root", repoRoot}, &out); err != nil {
		t.Fatal(err)
	}
	for _, tree := range []string{"mcp_e2e/", "mcp_e2e_rest/"} {
		if !strings.Contains(out.String(), tree) {
			t.Errorf("the report names no %s folder:\n%s", tree, out.String())
		}
	}
}

func TestStampsAreWrittenPerDoor(t *testing.T) {
	declareTree(t)
	stamps := map[string]map[string]map[string]json.RawMessage{}
	for _, door := range []string{"mcp", "rest"} {
		path := filepath.Join(t.TempDir(), "stamps.json")
		if err := run([]string{"stamps", "-root", repoRoot, "-door", door, "-out", path}, &strings.Builder{}); err != nil {
			t.Fatalf("%s: %v", door, err)
		}
		body, err := os.ReadFile(path) // #nosec G304 -- t.TempDir() + a literal filename, test-only
		if err != nil {
			t.Fatal(err)
		}
		byName := map[string]map[string]json.RawMessage{}
		if err := json.Unmarshal(body, &byName); err != nil {
			t.Fatal(err)
		}
		stamps[door] = byName
	}
	for name, mcp := range stamps["mcp"] {
		rest := stamps["rest"][name]
		if string(mcp["scenario"]) != string(rest["scenario"]) || string(mcp["tools"]) == string(rest["tools"]) {
			t.Errorf("%s: one scenario through two doors must share its scenario digest and differ in tool copy: %v %v",
				name, mcp, rest)
		}
	}
}

func TestATreeAgainstItselfMakesNothingStale(t *testing.T) {
	declareTree(t)
	var out strings.Builder
	if err := run([]string{"newly-stale", "-root", repoRoot, "-base", repoRoot}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("a tree against itself listed stale verdicts:\n%s", out.String())
	}
}

func TestAMisusedCommandSaysWhy(t *testing.T) {
	for _, args := range [][]string{nil, {"stamps"}, {"newly-stale"}, {"made-up"}, {"report", "-no-such-flag"}} {
		if err := run(args, &strings.Builder{}); err == nil {
			t.Errorf("%v ran without an error", args)
		}
	}
}
