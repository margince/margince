// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"testing"
)

const repoRoot = "../../.."

func TestTheCommittedRulesMatchTheirPolicies(t *testing.T) {
	units, err := unitsFrom(repoRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(units) < 2 {
		t.Fatalf("found %d units with a policy, want the EU and Asia-Pacific units at least: %v", len(units), units)
	}
	if err := run(repoRoot, "", true); err != nil {
		t.Fatal(err)
	}
}

// unit copies one shipped pack into a temporary folder the test may change.
func unit(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(repoRoot, "extensions", name))); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCheckFailsAStaleGeneratedFile(t *testing.T) {
	dir := unit(t, "apac")
	target := filepath.Join(dir, "messagingrules_gen.go")
	if err := os.WriteFile(target, []byte("package apac\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run("", dir, true); err == nil {
		t.Fatal("-check passed a generated file that differs from its policy")
	}
	if err := run("", dir, false); err != nil {
		t.Fatal(err)
	}
	if err := run("", dir, true); err != nil {
		t.Errorf("-check failed right after a generation: %v", err)
	}
}

// rootWith copies both shipped policy units into a temporary repository root
// and answers the one named. The other keeps the census from being empty.
func rootWith(t *testing.T, name string) (root, unit string) {
	t.Helper()
	root = t.TempDir()
	for _, name := range []string{"eu", "apac"} {
		if err := os.CopyFS(filepath.Join(root, "extensions", name), os.DirFS(filepath.Join(repoRoot, "extensions", name))); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(root, "", true); err != nil {
		t.Fatalf("the copied packs fail -check before anything is removed: %v", err)
	}
	return root, filepath.Join(root, "extensions", name)
}

func TestCheckFailsWhenAPackLosesPartOfItsPolicy(t *testing.T) {
	for name, remove := range map[string][]string{
		"the policy folder":                        {"policy"},
		"the generated file and the policy folder": {"messagingrules_gen.go", "policy"},
		"the go:generate file":                     {"gen.go"},
	} {
		root, unit := rootWith(t, "apac")
		for _, path := range remove {
			if err := os.RemoveAll(filepath.Join(unit, path)); err != nil {
				t.Fatal(err)
			}
		}
		if err := run(root, "", true); err == nil {
			t.Errorf("-check passed a pack without %s", name)
		}
	}
}

func TestCheckFailsWhenAPerCountryUnitLosesAPackOrItsSharedLayer(t *testing.T) {
	for name, remove := range map[string]string{
		"a country's index":     "policy/fr/index.md",
		"the shared EU layer":   "policy/regional/eu.md",
		"a country's own layer": "policy/fr/fr.md",
	} {
		root, unit := rootWith(t, "eu")
		if err := os.Remove(filepath.Join(unit, remove)); err != nil {
			t.Fatal(err)
		}
		if err := run(root, "", true); err == nil {
			t.Errorf("-check passed the eu unit without %s", name)
		}
	}
}

func TestCheckFailsAPerCountryUnitWhoseCountryDropsOut(t *testing.T) {
	root, unit := rootWith(t, "eu")
	if err := os.RemoveAll(filepath.Join(unit, "policy", "fr")); err != nil {
		t.Fatal(err)
	}
	if err := run(root, "", true); err == nil {
		t.Error("-check passed a generated file that still declares a country whose policy folder is gone")
	}
}
