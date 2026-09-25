// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// The two readers behind DeclaredPackageName, asked directly.
//
// They are unexported and their interesting answers are the ones the exported
// path cannot reach: a directory that is not there, a directory holding only
// tests, an import path outside this module. Each returns "no answer" rather
// than a wrong one, and a wrong one is what would make a census fail short —
// so these say that the no-answer is deliberate rather than incidental.

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGo(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// The reason the helper skips `_test.go` at all: a directory listing is ordered
// by name, so an external test package sorting FIRST would otherwise be handed
// back as the package's name. `a_test.go` sorts before `b.go` precisely so this
// fails without the skip.
func TestThePackageClauseComesFromProductionSourceNotAnExternalTest(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a_test.go", "package crmcontracts_test\n")
	writeGo(t, dir, "b.go", "package crmcontracts\n")

	name, ok := packageClauseIn(dir)

	if !ok {
		t.Fatal("a directory with a production file has a package name")
	}
	if name != "crmcontracts" {
		t.Errorf("read %q, want crmcontracts — the external test package was taken for the package's own name", name)
	}
}

// A directory holding only tests has no production package to name. False, not
// the test package's name: a wrong name sends a gate hunting a qualifier no
// file spells, which reports a clean package rather than a failure.
func TestADirectoryOfOnlyTestsNamesNoPackage(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "only_test.go", "package whatever_test\n")

	if name, ok := packageClauseIn(dir); ok {
		t.Errorf("read %q from a directory with no production source, want no answer", name)
	}
}

// A file that does not parse is skipped rather than fatal, and the next
// production file still answers. A half-written file in the working tree must
// not blind every gate that asks.
func TestAnUnparseableFileDoesNotHideTheOneBesideIt(t *testing.T) {
	dir := t.TempDir()
	writeGo(t, dir, "a_broken.go", "this is not Go at all {{{\n")
	writeGo(t, dir, "b_good.go", "package realname\n")

	name, ok := packageClauseIn(dir)

	if !ok || name != "realname" {
		t.Errorf("read (%q, %v), want realname — an unparseable neighbour hid the answer", name, ok)
	}
}

// A directory that is not there answers no, rather than panicking or inventing
// a name from the path.
func TestAMissingDirectoryNamesNoPackage(t *testing.T) {
	if name, ok := packageClauseIn(filepath.Join(t.TempDir(), "nothing-here")); ok {
		t.Errorf("read %q from a directory that does not exist", name)
	}
}

// An import path outside this module has no local directory to read. Standard
// library and third-party paths both land here, and the caller falls back to
// the path's last segment for them.
func TestAnImportPathOutsideTheModuleHasNoLocalDirectory(t *testing.T) {
	for _, path := range []string{"fmt", "github.com/example/other/pkg", ""} {
		if dir, ok := moduleLocalDir(path); ok {
			t.Errorf("%q resolved to local dir %q, want no answer — it is not in this module", path, dir)
		}
	}
}

// The module's own path resolves to the module root, which is the case the
// prefix test would get wrong if it only ever matched `modPath + "/"`.
func TestTheModulesOwnPathResolvesToItsRoot(t *testing.T) {
	root, modPath, ok := moduleRootAndPath()
	if !ok {
		t.Skip("no go.mod above the working directory; nothing to resolve against")
	}

	dir, resolved := moduleLocalDir(modPath)

	if !resolved || dir != root {
		t.Errorf("the module path resolved to (%q, %v), want the module root %q", dir, resolved, root)
	}
}
