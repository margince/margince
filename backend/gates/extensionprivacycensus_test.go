// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every table an extension's migrations create must carry a privacy declaration,
// and every declaration must name a table the migrations create.
//
// The boot compares the two against the migrated schema. This reads both from
// source, so a missing declaration fails in the pull request. Tables come from
// the unit's `*.up.sql` and the declaration from its `Privacy` field.

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

var (
	createdExtTable = regexp.MustCompile(`(?im)^\s*CREATE TABLE (?:IF NOT EXISTS )?ext\.(ext_\w+)`)
	droppedExtTable = regexp.MustCompile(`(?im)^\s*DROP TABLE (?:IF EXISTS )?ext\.(ext_\w+)`)
)

func TestEveryExtensionTableHasAPrivacyDeclaration(t *testing.T) {
	t.Parallel()
	tablesSeen := 0
	for _, u := range liveUnits(t) {
		dir := filepath.Join(tierDir, u.name)
		created := migratedTables(t, dir)
		declared := declaredPrivacyTables(t, dir)
		tablesSeen += len(created)
		for _, table := range created {
			if !slices.Contains(declared, table) {
				t.Errorf("extensions/%s creates %s and declares no privacy for it — add a privacy.Table naming "+
					"its links to core records, personal columns and retention class, and bump the unit's version", u.name, table)
			}
		}
		for _, table := range declared {
			if !slices.Contains(created, table) {
				t.Errorf("extensions/%s declares privacy for %s, which its migrations do not create", u.name, table)
			}
		}
	}
	if tablesSeen == 0 {
		t.Fatalf("no live unit creates an ext table, so this census read nothing; a moved tier or a changed " +
			"migration spelling would otherwise pass it")
	}
}

// migratedTables is the tables a unit's up migrations leave behind: created, less
// dropped. Comment lines are removed first, because the migrations explain
// themselves in prose that names statements.
func migratedTables(t *testing.T, unitDir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(unitDir, extension.MigrationsDir, "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var code []string
		for line := range strings.SplitSeq(string(raw), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				code = append(code, line)
			}
		}
		sql := strings.Join(code, "\n")
		for _, m := range createdExtTable.FindAllStringSubmatch(sql, -1) {
			tables = append(tables, m[1])
		}
		for _, m := range droppedExtTable.FindAllStringSubmatch(sql, -1) {
			tables = slices.DeleteFunc(tables, func(name string) bool { return name == m[1] })
		}
	}
	slices.Sort(tables)
	return slices.Compact(tables)
}

// declaredPrivacyTables reads the Name of every element of a unit's Privacy field.
func declaredPrivacyTables(t *testing.T, unitDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(unitDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		ast.Inspect(parseGo(t, filepath.Join(unitDir, e.Name())), func(n ast.Node) bool {
			field, ok := n.(*ast.KeyValueExpr)
			if !ok || !isIdentNamed(field.Key, "Privacy") {
				return true
			}
			list, ok := field.Value.(*ast.CompositeLit)
			if !ok {
				return false
			}
			for _, elt := range list.Elts {
				if table, ok := elt.(*ast.CompositeLit); ok {
					names = append(names, compositeStringField(table, "Name"))
				}
			}
			return false
		})
	}
	slices.Sort(names)
	return names
}

// compositeStringField is the string literal a composite literal sets for one
// key, or empty when the key is absent or not a literal.
func compositeStringField(lit *ast.CompositeLit, key string) string {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok || !isIdentNamed(kv.Key, key) {
			continue
		}
		if basic, ok := kv.Value.(*ast.BasicLit); ok {
			if s, err := strconv.Unquote(basic.Value); err == nil {
				return s
			}
		}
	}
	return ""
}
