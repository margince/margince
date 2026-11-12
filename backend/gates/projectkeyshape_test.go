// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

package gates

// A project key has one shape, projectkey.CheckPattern, and Go reads it through
// projectkey.Shaped. The contract, the frontend's subject-tag check and the
// generated pages cannot import Go, so they repeat it. Each such mirror is
// declared here and carries the pattern as written. Any other file that spells
// a key-shaped pattern, in any letter case, is a second answer.

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/projectkey"
)

// projectKeyShapeFingerprint finds a key-shaped pattern however it is
// spelled: the bounded run of letters, digits, underscore and hyphen.
var projectKeyShapeFingerprint = regexp.MustCompile(`(?i)0-9_-\]\{1,23\}`)

// projectKeyShapeMirrors are the files that repeat the pattern, paths from
// backend/. The owner is first.
var projectKeyShapeMirrors = []string{
	"internal/shared/kernel/projectkey/projectkey.go",
	"api/crm.yaml",
	"internal/compose/agentbundle/agentcontract_gen.yaml",
	"../frontend/src/features/projects/projectrecord.ts",
	"../docs/reference/entity-model/projects.md",
}

func TestTheProjectKeyShapeHasOneOwnerAndDeclaredMirrors(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	files := 0
	for _, root := range []string{".", "../frontend/src", "../extensions", "../docs"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// Dependencies and build output are not the tree's own source.
				if d.Name() == "node_modules" || (d.Name() != "." && strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !slices.Contains([]string{".go", ".ts", ".tsx", ".yaml", ".md"}, filepath.Ext(path)) {
				return nil
			}
			files++
			body, err := gatekit.ReadFile(path)
			if err != nil {
				return err
			}
			if projectKeyShapeFingerprint.Match(body) {
				found[filepath.ToSlash(path)] = !strings.Contains(string(body), projectkey.CheckPattern)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	if files < 3000 {
		t.Fatalf("read %d files; the census went short", files)
	}
	delete(found, "gates/projectkeyshape_test.go")
	for _, mirror := range projectKeyShapeMirrors {
		stale, present := found[mirror]
		switch {
		case !present:
			t.Errorf("%s no longer spells the key shape; drop it from projectKeyShapeMirrors", mirror)
		case stale:
			t.Errorf("%s spells a key shape that is not projectkey.CheckPattern %q", mirror, projectkey.CheckPattern)
		}
		delete(found, mirror)
	}
	for path := range found {
		t.Errorf("%s spells the project key shape again; read projectkey.Shaped, or declare it a mirror here", path)
	}
}
