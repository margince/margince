// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

//go:build !integration

package gates

import (
	"go/ast"
	"go/parser"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestApprovalConstructionUsesTheContextBoundFactory(t *testing.T) {
	t.Parallel()
	constructors := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		calls := approvalConstructorCalls(file)
		constructors += calls
		if calls == 0 {
			continue
		}
		if filepath.ToSlash(path) != "internal/compose/approvalservice.go" {
			t.Errorf("%s constructs approvals without the shared context-bound factory", path)
		}
		bound := false
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if ok && method.Sel.Name == "WithPreview" && len(call.Args) == 1 {
				bound = true
			}
			return true
		})
		if !bound {
			t.Errorf("%s does not bind decision previews", path)
		}
	}
	if constructors != 1 {
		t.Fatalf("found %d approval constructors, want the shared factory", constructors)
	}
}

func approvalConstructorCalls(file *ast.File) int {
	aliases := map[string]bool{}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || path != "github.com/margince/margince/backend/internal/modules/approvals" {
			continue
		}
		name := "approvals"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		aliases[name] = true
	}
	count := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if direct, ok := node.(*ast.Ident); ok && aliases["."] && direct.Name == "NewService" {
			count++
			return true
		}
		method, ok := node.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "NewService" {
			return true
		}
		pkg, ok := method.X.(*ast.Ident)
		if ok && aliases[pkg.Name] {
			count++
		}
		return true
	})
	return count
}

func TestApprovalConstructorCensusFindsImportAndFunctionAliases(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`package probe; import a "github.com/margince/margince/backend/internal/modules/approvals"; var service = a.NewService(nil)`,
		`package probe; import a "github.com/margince/margince/backend/internal/modules/approvals"; var factory = a.NewService`,
		`package probe; import . "github.com/margince/margince/backend/internal/modules/approvals"; var service = NewService(nil)`,
	} {
		file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := approvalConstructorCalls(file); got != 1 {
			t.Fatalf("found %d constructors in %s", got, source)
		}
	}
}
