// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"os"
	"path/filepath"
	"testing"
)

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
}
