// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every core read of a unit table carries the kind's row scope, or says why it
// may not. A reader without it still answers plausible rows, a colleague's ones
// too. The corpus is every function under internal/ that spells the ext schema,
// directly or through a constant of any package, whatever the file imports.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// extensionScopeCalls render the kind's row scope into a read.
var extensionScopeCalls = []string{"ExtensionScopeClause", "ExtensionTableScopeClause"}

// unscopedExtensionReaders name the ext schema without rendering the scope.
// Each reason says where the scope is applied instead, or why no reader is
// served a row.
var unscopedExtensionReaders = gatekit.Waive(map[string]string{
	"compose/reportjoin.Resolve":                "builds reportSpec.extensionJoin; specScopeClauses always calls Join.Scope before executing the joined report",
	"collections.extensionEngine":               "builds a storekit.Query with Object set; Query.rowScope renders the kind's scope on every execution",
	"compose.extensionKindSpec":                 "builds a reportSpec with extensionKind set; specScopeClauses renders the kind's scope for every report",
	"compose.extensionHistoryQuery":             "its one caller, extensionRecordEntries, renders the kind's scope and passes it in",
	"compose.VerifyRecordKinds":                 "reads the table's catalog at boot, never a row",
	"compose/corereach.Install":                 "names the tables a unit owns for the reach check, never a row",
	"platform/sweepreach.Install":               "names the tables a unit owns for the reach check, never a row",
	"platform/extprivacy.qualified":             "spells a table name for the erasure engine, which runs as the system and must clear every row of a data subject",
	"platform/extprivacy.CountDependents":       "counts the unit rows a merge would leave behind; the merge refuses on any of them, whoever may read it",
	"platform/extprivacy.clearAssignments":      "reads the column types of a table it clears for an erasure, never a row",
	"platform/extprivacy.VerifySchema":          "reads the catalog of a unit's tables at boot, never a row",
	"platform/extprivacy.readPinningReferences": "reads the catalog of a unit's tables at boot, never a row",
	"platform/extprivacy.readTables":            "reads the catalog of a unit's tables at boot, never a row",
})

// extensionSchemaReaders is how many functions name the ext schema. The census
// fails on fewer as well as more, so a reader that stops being found is seen.
const extensionSchemaReaders = 17

func TestEveryReadOfAnExtensionKindCarriesItsRowScope(t *testing.T) {
	t.Parallel()
	defer unscopedExtensionReaders.AssertAllMatched(t)
	root := filepath.Join(gatekit.ModuleRoot(t), "internal")
	dirs := packageDirs(t, root)
	consts := map[string]map[string]string{}
	byName := map[string]string{}
	for _, dir := range dirs {
		consts[dir] = gatekit.PackageStringConstants(t, dir)
		byName[filepath.Base(dir)] = dir
	}
	var found []string
	for _, dir := range dirs {
		pkg, err := filepath.Rel(root, dir)
		if err != nil {
			t.Fatal(err)
		}
		pkg = strings.TrimPrefix(pkg, "modules/")
		for _, fn := range extSchemaFunctions(t, dir, consts, byName) {
			subject := pkg + "." + fn.Name.Name
			found = append(found, subject)
			if gatekit.CallsAny(fn.Body, extensionScopeCalls) || unscopedExtensionReaders.Waived(t, subject) {
				continue
			}
			t.Errorf("%s reads a table in the ext schema without the kind's row scope; "+
				"add auth.ExtensionScopeClause to its WHERE, or waive it here naming where the scope is applied", subject)
		}
	}
	if len(found) != extensionSchemaReaders {
		sort.Strings(found)
		t.Errorf("the census found %d functions naming the ext schema, want %d: %v. "+
			"Check each new or missing one, then set extensionSchemaReaders", len(found), extensionSchemaReaders, found)
	}
}

func packageDirs(t *testing.T, root string) []string {
	t.Helper()
	var dirs []string
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			dirs = append(dirs, path)
		}
		return err
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return dirs
}

// extSchemaFunctions are the functions of one package directory whose body
// spells the ext schema.
func extSchemaFunctions(t *testing.T, dir string, consts map[string]map[string]string, byName map[string]string) []*ast.FuncDecl {
	t.Helper()
	sources, err := gatekit.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	value := func(node ast.Node) string {
		switch n := node.(type) {
		case *ast.BasicLit:
			if unquoted, err := strconv.Unquote(n.Value); err == nil && n.Kind == token.STRING {
				return unquoted
			}
		case *ast.Ident:
			return consts[dir][n.Name]
		case *ast.SelectorExpr:
			if pkg, ok := n.X.(*ast.Ident); ok {
				return consts[byName[pkg.Name]][n.Sel.Name]
			}
		}
		return ""
	}
	var out []*ast.FuncDecl
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(source, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", source, err)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && spellsExtSchema(fn.Body, value) {
				out = append(out, fn)
			}
		}
	}
	return out
}

func spellsExtSchema(body ast.Node, value func(ast.Node) string) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if v := value(node); v == "ext" || v == `"ext".` {
			found = true
		}
		return !found
	})
	return found
}
