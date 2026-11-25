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

func TestATreeAgainstItsOwnAnswerMakesNothingStale(t *testing.T) {
	declareTree(t)
	current := filepath.Join(t.TempDir(), "current.json")
	if err := run([]string{"current", "-root", repoRoot, "-out", current}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := run([]string{"newly-stale", "-root", repoRoot, "-base-current", current}, &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("a tree against its own answer listed stale verdicts:\n%s", out.String())
	}
}

// Every verdict the base found current is newly stale under a head that finds
// it stale, so the table names each one.
func TestABaseThatFoundEveryVerdictCurrentListsEachStaleOne(t *testing.T) {
	declareTree(t)
	dir := t.TempDir()
	current := filepath.Join(dir, "current.json")
	if err := run([]string{"current", "-root", repoRoot, "-out", current}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(current) // #nosec G304 -- t.TempDir() + a literal filename, test-only
	if err != nil {
		t.Fatal(err)
	}
	currency := map[string]bool{}
	if err := json.Unmarshal(body, &currency); err != nil {
		t.Fatal(err)
	}
	stale := 0
	for key, isCurrent := range currency {
		if !isCurrent {
			stale++
		}
		currency[key] = true
	}
	allCurrent := filepath.Join(dir, "all-current.json")
	writeFile(t, allCurrent, currency)
	var out strings.Builder
	if err := run([]string{"newly-stale", "-root", repoRoot, "-base-current", allCurrent}, &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), "\n| `"); got != stale {
		t.Errorf("listed %d verdicts, want the %d this tree finds stale:\n%s", got, stale, out.String())
	}
}

func writeFile(t *testing.T, path string, value map[string]bool) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAMisusedCommandSaysWhy(t *testing.T) {
	dir := t.TempDir()
	notJSON, null := filepath.Join(dir, "not.json"), filepath.Join(dir, "null.json")
	writeFile(t, null, nil)
	if err := os.WriteFile(notJSON, []byte("stale: yes"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		nil, {"stamps"}, {"current"}, {"newly-stale"}, {"made-up"}, {"report", "-no-such-flag"},
		{"current", "-root", dir, "-out", filepath.Join(dir, "current.json")},
		{"newly-stale", "-base-current", filepath.Join(dir, "missing.json")},
		{"newly-stale", "-base-current", notJSON},
		{"newly-stale", "-base-current", null},
	} {
		if err := run(args, &strings.Builder{}); err == nil {
			t.Errorf("%v ran without an error", args)
		}
	}
}
