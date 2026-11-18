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

func TestCheckFailsAGeneratedFileWhosePolicyIsGone(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "extensions", "vn")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join(repoRoot, "extensions", "vn"))); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "policy")); err != nil {
		t.Fatal(err)
	}
	if err := run(root, "", true); err == nil {
		t.Fatal("-check passed a generated file with no policy behind it")
	}
}
