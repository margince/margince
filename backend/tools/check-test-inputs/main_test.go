// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// plantedRepository builds a repository holding one module, and answers the
// package directory the logs below ran in and what --names says for the tree.
func plantedRepository(t *testing.T) (string, map[string]string) {
	t.Helper()
	repo := t.TempDir()
	pkg := filepath.Join(repo, "backend", "internal", "thing")
	for _, dir := range []string{filepath.Join(repo, ".git"), pkg, filepath.Join(repo, "docs")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "backend", "go.mod"), []byte("module example.com/backend\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return pkg, map[string]string{"docs": "TREE_DIGEST_DOCS", "backend": "TREE_DIGEST_BACKEND"}
}

func TestAReadOfAnUntrackedEntryIsReportedAsUndescribed(t *testing.T) {
	pkg, plantedNames := plantedRepository(t)
	findings, err := undeclaredReads(strings.NewReader("getenv TREE_DIGEST_BUILD\nopen ../../../build/out.json\n"), pkg, plantedNames)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0], "no tree digest describes") {
		t.Errorf("a read of the untracked build/ produced %q, want one finding saying no digest describes it", findings)
	}
}

func TestAReadOutsideTheModuleWithoutItsDigestIsReported(t *testing.T) {
	pkg, plantedNames := plantedRepository(t)
	findings, err := undeclaredReads(strings.NewReader("# test log\nopen ../../../docs/page.md\n"), pkg, plantedNames)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || !strings.Contains(findings[0], "docs/page.md") || !strings.Contains(findings[0], "TREE_DIGEST_DOCS") {
		t.Errorf("an undeclared read of docs/page.md produced %q, want one finding naming it and TREE_DIGEST_DOCS", findings)
	}
}

func TestADeclaredReadAndReadsTheCacheSeesAreNotReported(t *testing.T) {
	pkg, plantedNames := plantedRepository(t)
	for _, tc := range []struct{ name, log string }{
		{"a read whose digest the test read", "getenv TREE_DIGEST_DOCS\nopen ../../../docs/page.md\n"},
		{"a read inside the module", "open ../other/file.go\nstat ../../go.mod\n"},
		{"a read outside the repository", "open /etc/hosts\n"},
		{"git's own bookkeeping", "stat ../../../.git\n"},
		{"a relative read after a chdir into the module", "chdir ../..\nopen internal/thing/x.go\n"},
	} {
		findings, err := undeclaredReads(strings.NewReader(tc.log), pkg, plantedNames)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(findings) != 0 {
			t.Errorf("%s: reported %q, want nothing", tc.name, findings)
		}
	}
}

func TestACensusOverAnEmptyDirectoryReadsNothing(t *testing.T) {
	findings, read, err := census(t.TempDir(), map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if read != 0 || len(findings) != 0 {
		t.Errorf("an empty log directory read %d logs with %d findings, want none — main's floor is what refuses it", read, len(findings))
	}
}
