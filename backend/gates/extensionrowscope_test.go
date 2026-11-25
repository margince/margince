// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every core read of an extension kind's rows carries the kind's row scope. A
// reader that names a table in the ext schema and forgets the scope still
// answers plausible rows, only a colleague's ones too. The corpus is every
// function under internal/ that names the ext schema in a file that imports the
// record-kind registry, so a new reader is read without being listed here. A
// file without that import cannot tell a kind's table from the unit's others:
// those are the privacy and reach plumbing, which run as the system.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// extensionScopeCarriers are the names that put the kind's row scope on a read.
// ExtensionScopeClause renders it. A storekit.Query with Object set and a
// reportSpec with extensionKind set have it rendered by their one executor.
const recordKindRegistry = "github.com/margince/margince/backend/internal/shared/ports/recordkinds"

var extensionScopeCarriers = map[string]bool{"ExtensionScopeClause": true, "Object": true, "extensionKind": true}

// unscopedExtensionReaders name the ext schema and return no row to a reader.
var unscopedExtensionReaders = gatekit.Waive(map[string]string{
	"compose.VerifyRecordKinds":     "reads the table's catalog at boot, never a row",
	"compose.extensionHistoryQuery": "its one caller, extensionRecordEntries, renders the kind's row scope and passes it in",
})

func TestEveryReadOfAnExtensionKindCarriesItsRowScope(t *testing.T) {
	t.Parallel()
	defer unscopedExtensionReaders.AssertAllMatched(t)
	root := filepath.Join(gatekit.ModuleRoot(t), "internal")
	seen := 0
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return err
		}
		seen += extensionReadersIn(t, root, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if seen < 7 {
		t.Fatalf("the census found %d functions naming the ext schema, want at least the 7 this gate was written against: the spelling of the schema changed", seen)
	}
}

// extensionReadersIn checks each function of one package directory that names
// the ext schema, and returns how many it found.
func extensionReadersIn(t *testing.T, root, dir string) int {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil || len(sources) == 0 {
		return 0
	}
	consts := gatekit.PackageStringConstants(t, dir)
	pkg, err := filepath.Rel(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	pkg = strings.TrimPrefix(strings.TrimPrefix(pkg, "modules/"), "platform/")
	found := 0
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(source, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		if qualifier, dotted := gatekit.ImportedAs(file, recordKindRegistry); qualifier == "" && !dotted {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !namesExtSchema(fn.Body, consts) {
				continue
			}
			found++
			if carriesExtensionScope(fn.Body) || unscopedExtensionReaders.Waived(t, pkg+"."+fn.Name.Name) {
				continue
			}
			t.Errorf("%s: %s reads a table in the ext schema without the kind's row scope; "+
				"add auth.ExtensionScopeClause to its WHERE, or read through a storekit.Query with Object set",
				source, fn.Name.Name)
		}
	}
	return found
}

// namesExtSchema reports a body that spells the ext schema: the literal "ext"
// or `"ext".`, directly or through a package constant.
func namesExtSchema(body ast.Node, consts map[string]string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		var value string
		switch n := node.(type) {
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				return true
			}
			unquoted, err := strconv.Unquote(n.Value)
			if err != nil {
				return true
			}
			value = unquoted
		case *ast.Ident:
			value = consts[n.Name]
		}
		if value == "ext" || value == `"ext".` {
			found = true
		}
		return !found
	})
	return found
}

// carriesExtensionScope reports a body that names one of the scope carriers, as
// a call or as a struct field it sets.
func carriesExtensionScope(body ast.Node) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			found = found || n.Sel.Name == "ExtensionScopeClause"
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "ExtensionScopeClause" {
				found = true
			}
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok && extensionScopeCarriers[key.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}
