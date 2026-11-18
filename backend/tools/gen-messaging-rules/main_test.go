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
		t.Fatalf("found %d units with a policy, want the German and Vietnamese packs at least: %v", len(units), units)
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
	dir := unit(t, "vn")
	target := filepath.Join(dir, "messagingrules_gen.go")
	if err := os.WriteFile(target, []byte("package vn\n"), 0o600); err != nil {
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

// rootWithVN copies both shipped packs into a temporary repository root and
// answers the Vietnamese one. The German one keeps the census from being empty.
func rootWithVN(t *testing.T) (root, unit string) {
	t.Helper()
	root = t.TempDir()
	for _, name := range []string{"de", "vn"} {
		if err := os.CopyFS(filepath.Join(root, "extensions", name), os.DirFS(filepath.Join(repoRoot, "extensions", name))); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(root, "", true); err != nil {
		t.Fatalf("the copied packs fail -check before anything is removed: %v", err)
	}
	return root, filepath.Join(root, "extensions", "vn")
}

func TestCheckFailsWhenAPackLosesPartOfItsPolicy(t *testing.T) {
	for name, remove := range map[string][]string{
		"the policy folder":                        {"policy"},
		"the generated file and the policy folder": {"messagingrules_gen.go", "policy"},
		"the index":            {"policy/index.md"},
		"the go:generate file": {"gen.go"},
	} {
		root, unit := rootWithVN(t)
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
