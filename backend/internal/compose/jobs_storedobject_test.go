// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/storedobject"
)

// storedObjectDeclarer is the function a module declares its stored-object
// kind with.
const storedObjectDeclarer = "StoredObjectReference"

// TestEveryModulesStoredObjectReferenceIsReaped holds StoredObjectReferences to
// the tree: a module that declares a kind the reap is not handed keeps every
// orphan of that kind forever, and nothing else would notice.
func TestEveryModulesStoredObjectReferenceIsReaped(t *testing.T) {
	t.Parallel()
	declaring := modulesDeclaringStoredObjects(t)
	refs := StoredObjectReferences()
	// One per declaring module, and every kind distinct: a module listed twice
	// repeats its kind, which NewLedger refuses, so equal counts mean each
	// declaring module is listed exactly once.
	if len(refs) != len(declaring) {
		t.Errorf("StoredObjectReferences hands the reap %d declarations; these modules declare one: %v",
			len(refs), declaring)
	}
	if _, err := storedobject.NewLedger(nil, refs...); err != nil {
		t.Errorf("the collected declarations do not form a ledger: %v", err)
	}
}

// modulesDeclaringStoredObjects walks every module for a package-level
// StoredObjectReference function.
func modulesDeclaringStoredObjects(t *testing.T) []string {
	t.Helper()
	found := map[string]bool{}
	root := filepath.Join("..", "modules")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			if fn, isFunc := decl.(*ast.FuncDecl); isFunc && fn.Recv == nil && fn.Name.Name == storedObjectDeclarer {
				found[filepath.ToSlash(filepath.Dir(path))] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	modules := make([]string, 0, len(found))
	for dir := range found {
		modules = append(modules, dir)
	}
	sort.Strings(modules)
	return modules
}
