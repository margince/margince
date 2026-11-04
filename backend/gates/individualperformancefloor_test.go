// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// The small-group floor of "Show individual performance" is compared in one
// function, belowFloor, and nowhere else.
//
// Every surface reaches the floor through thinGroups. A second comparison with
// fewestColleagues is a second floor, which can drift from the first. The
// constant is unexported, so the compose package's files are the corpus.

import (
	"go/ast"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	floorConstant = "fewestColleagues"
	floorFunction = "belowFloor"
)

func TestTheIndividualPerformanceFloorIsComparedInOneFunction(t *testing.T) {
	t.Parallel()

	const root = "internal/compose"
	fset := gatekit.SourceFileSet()
	declared, permitted := 0, 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := gatekit.ParseFile(filepath.ToSlash(path), 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range file.Decls {
			owner, isFunc := "", false
			if fn, ok := decl.(*ast.FuncDecl); ok {
				owner, isFunc = fn.Name.Name, true
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				spec, ok := n.(*ast.ValueSpec)
				if ok && !isFunc && len(spec.Names) == 1 && spec.Names[0].Name == floorConstant {
					declared++
					return false
				}
				id, ok := n.(*ast.Ident)
				if !ok || id.Name != floorConstant {
					return true
				}
				if owner == floorFunction {
					permitted++
					return true
				}
				t.Errorf("%s: %s reads %s. The floor is compared only in %s, which every "+
					"surface reaches through thinGroups; ask thinGroups instead of comparing a "+
					"count here.", fset.Position(id.Pos()), describeOwner(owner), floorConstant, floorFunction)
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if declared != 1 || permitted == 0 {
		t.Fatalf("found %d declarations of %s and %d reads in %s; want one of each at least. "+
			"The gate has stopped reaching the floor it guards", declared, floorConstant, permitted, floorFunction)
	}
}

func describeOwner(owner string) string {
	if owner == "" {
		return "a package-level declaration"
	}
	return owner
}
