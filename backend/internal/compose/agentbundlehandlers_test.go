// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/agentbundle"
)

// A handler that calls auth.RequireHuman refuses every passport, whatever the
// contract says. The skill must not list its operation, or an agent is handed
// a call it can only fail. The generated router calls the operationId with its
// first letter raised, so the handlers are found by that name across
// internal/, the stubs aside.
func TestTheSkillListsNoOperationWhoseHandlerRequiresAHuman(t *testing.T) {
	archive, err := (&agentbundle.Builder{}).Build("https://crm.example.test/v1")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	listed := map[string]string{}
	for id := range operationIDsIn(t, bundledSpec(t, archive)) {
		listed[strings.ToUpper(id[:1])+id[1:]] = id
	}

	found, humanOnly := map[string]bool{}, map[string]string{}
	err = filepath.WalkDir("..", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "stubs_gen.go") {
			return walkErr
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil {
				continue
			}
			if _, isListed := listed[fn.Name.Name]; !isListed {
				continue
			}
			found[fn.Name.Name] = true
			if callsRequireHuman(fn.Body) {
				humanOnly[fn.Name.Name] = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scanning internal/: %v", err)
	}

	missing := map[string]bool{}
	for name, id := range listed {
		if !found[name] {
			missing[id] = true
		}
	}
	if len(missing) > 0 {
		t.Fatalf("found no handler for %v; the scan is short, so a pass would prove nothing", sortedIDs(missing))
	}
	for name, path := range humanOnly {
		t.Errorf("the skill lists %s, and its handler in %s calls auth.RequireHuman, so a passport is always refused",
			listed[name], path)
	}
}

func callsRequireHuman(body *ast.BlockStmt) bool {
	calls := false
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "RequireHuman" {
			calls = true
		}
		return !calls
	})
	return calls
}
