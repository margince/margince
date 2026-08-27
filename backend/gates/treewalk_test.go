// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The one walk over this repository's hand-written trees.
//
// Two gates used to carry their own: the license header sweep and the env-var
// contract, over the same roots, with the same node_modules skip, the same
// regular-file guard and the same generated-file filter. Nothing was wrong
// with either, and that is the hazard — a duplicated walk drifts one skip at a
// time, and the gate that keeps the older copy quietly covers less than the
// one that got the fix.
//
// What was already shared is the ROOTS (licensedTrees, license_test.go), so a
// new Go tree enrols in every gate at once. This is the loop around that list.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// walkTextFiles reads every file under root and hands each to visit. The trees
// swept here are small and text-only; a read error fails the test rather than
// narrowing the sweep in silence.
func walkTextFiles(t *testing.T, root string, visit func(path, text string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// A unit's frontend layer is a pnpm workspace package, so extensions/
		// now contains an installed dependency tree — thousands of files this
		// sweep has no business reading, and symlinked DIRECTORIES that are not
		// IsDir() and so reach the ReadFile below as "is a directory".
		if d.IsDir() && d.Name() == "node_modules" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		// Belt and braces for the same reason: a symlink is not a regular file,
		// and this sweep reads text a human wrote.
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path) // #nosec G304 G122 -- path comes from walking a fixed root inside the trusted source tree
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(path), string(b))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// walkHandWrittenGoFiles visits the Go files a human wrote under one root, and
// answers how many it found.
//
// The COUNT is the reason this is a wrapper rather than a filter each caller
// repeats: a gate whose root yields nothing passes exactly like a clean one,
// and only the caller knows whether an empty root is legitimate (extensions/
// is; backend/ is not). Handing back the number lets each decide, without
// either of them re-deriving what "hand-written Go file" means.
func walkHandWrittenGoFiles(t *testing.T, root string, visit func(path, text string)) int {
	t.Helper()
	found := 0
	walkTextFiles(t, root, func(path, text string) {
		if !strings.HasSuffix(path, ".go") || isGenerated(path, text) {
			return
		}
		found++
		visit(path, text)
	})
	return found
}
