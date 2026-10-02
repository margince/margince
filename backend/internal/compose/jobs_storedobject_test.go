// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
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
	collected := modulesCollectedForTheReap(t)
	// Matched module by module, so a declaration left out cannot be hidden by
	// some other reference standing in its slot.
	if !slices.Equal(collected, declaring) {
		t.Errorf("StoredObjectReferences collects the declarations of %v; these modules declare one: %v",
			collected, declaring)
	}
	if refs := StoredObjectReferences(); len(refs) != len(collected) {
		t.Errorf("StoredObjectReferences hands the reap %d declarations but collects %d modules' — every "+
			"element must be a module's %s()", len(refs), len(collected), storedObjectDeclarer)
	}
	if _, err := storedobject.NewLedger(nil, StoredObjectReferences()...); err != nil {
		t.Errorf("the collected declarations do not form a ledger: %v", err)
	}
}

// modulesDeclaringStoredObjects walks every module for a package-level
// StoredObjectReference function, and answers each as modules/<dir>.
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
				found[strings.TrimPrefix(filepath.ToSlash(filepath.Dir(path)), "../")] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return sortedKeysOf(found, strings.Compare)
}

// modulesCollectedForTheReap reads StoredObjectReferences' source and answers
// the module of each `<module>.StoredObjectReference()` it returns, as
// modules/<dir>.
func modulesCollectedForTheReap(t *testing.T) []string {
	t.Helper()
	const source = "jobs_storedobject.go"
	file, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", source, err)
	}
	imports := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("reading an import of %s: %v", source, err)
		}
		name := filepath.Base(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = path
	}
	found := map[string]bool{}
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Name.Name != "StoredObjectReferences" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == storedObjectDeclarer {
				if qualifier, isIdent := sel.X.(*ast.Ident); isIdent {
					if _, module, cut := strings.Cut(imports[qualifier.Name], "/internal/"); cut {
						found[module] = true
					}
				}
			}
			return true
		})
	}
	if len(found) == 0 {
		t.Fatalf("no %s() call found in StoredObjectReferences in %s: the read has gone blind", storedObjectDeclarer, source)
	}
	return sortedKeysOf(found, strings.Compare)
}
