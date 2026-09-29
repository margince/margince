// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnExistingFileAndANestedTreeAreDeclared(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "input.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := DeclareInputs(root, file); err != nil {
		t.Errorf("declaring a readable tree and a file it holds failed: %v", err)
	}
	if err := DeclareListings(root, nested); err != nil {
		t.Errorf("declaring two readable directories' listings failed: %v", err)
	}
}

func TestATreeDigestVariableIsNamedForItsEntry(t *testing.T) {
	for top, want := range map[string]string{
		"docs":       "TREE_DIGEST_DOCS",
		".gitignore": "TREE_DIGEST__GITIGNORE",
		"go.work":    "TREE_DIGEST_GO_WORK",
		"user-guide": "TREE_DIGEST_USER_GUIDE",
	} {
		if got := TreeDigestVar(top); got != want {
			t.Errorf("TreeDigestVar(%q) = %q, want %q", top, got, want)
		}
	}
}

func TestAnInputThatDoesNotExistYetIsDeclaredWithoutError(t *testing.T) {
	if err := DeclareInputs(filepath.Join(t.TempDir(), ".gitignore")); err != nil {
		t.Errorf("a missing ignore file is an input whose creation must invalidate, not an error: %v", err)
	}
}

func TestATreeTheWalkCannotReadIsAnErrorNotAPartialDeclaration(t *testing.T) {
	root := t.TempDir()
	sealed := filepath.Join(root, "sealed")
	if err := os.Mkdir(sealed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(sealed, 0o700); err != nil {
			t.Errorf("unsealing %s for cleanup: %v", sealed, err)
		}
	})
	if _, err := os.ReadDir(sealed); err == nil {
		t.Skip("this user reads a mode-000 directory (root), so the walk cannot be made to fail")
	}
	if err := DeclareInputs(root); err == nil {
		t.Error("a walk that could not list a directory declared the tree anyway, leaving the cache blind to it")
	}
	if err := DeclareListings(sealed); err == nil {
		t.Error("a listing that could not be read was declared anyway, leaving the cache blind to it")
	}
}
