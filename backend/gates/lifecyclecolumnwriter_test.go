// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A table that declares a lifecycle column has a statement that updates its rows.
//
// `version` and `updated_at` describe a row that changes. On a table whose rows
// are only inserted, they repeat the insert forever. And `version` tells the
// next author that a guarded update path exists when none does.
//
// The corpus is every table the head catalog gives one of the two columns. The
// evidence is a production statement that updates the table. That is an UPDATE,
// an upsert that updates, a storekit row writer, or a migration's SQL function.
// A data fix in a migration runs once, so it does not count.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// lifecycleColumn reads a catalog line declaring an integer version counter or
// an updated_at timestamp. A text `version` names a wording, not a counter.
var lifecycleColumn = regexp.MustCompile(`^public\.(\w+)\.(version (?:bigint|integer)|updated_at timestamp)`)

// lifecycleTableFloor is the vacuity floor: the catalog held 130 such tables
// when this gate was written, and far fewer means the reader stopped matching.
const lifecycleTableFloor = 100

// rowWritingCalls are the storekit entry points whose table argument names a
// row they update or lock for an update, with that argument's position.
var rowWritingCalls = map[string]int{
	"ApplyWithVersion": 2, "ApplyGuarded": 2, "LockRow": 2, "LockPair": 2,
	"ClaimOwnership": 2, "EnsureIfMatch": 2, "EnsureReleasedVersion": 2,
}

var (
	updateTarget = regexp.MustCompile(`(?is)\bUPDATE\s+(?:ONLY\s+)?(?:public\.)?(\w+)`)
	upsertTarget = regexp.MustCompile(`(?is)\bINSERT\s+INTO\s+(?:public\.)?(\w+)\b.*?\bON\s+CONFLICT\b.*?\bDO\s+UPDATE\b`)
	// sqlFunction is one function a migration installs, with its body.
	sqlFunction = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\b.*?\bAS\s+(\$\w*\$)(.*?)\$\w*\$`)
)

// appendOnlyLifecycle names each table that keeps a lifecycle column although
// no statement updates its rows, with why.
var appendOnlyLifecycle = gatekit.Waive(map[string]string{
	"deal_product": "Each write deletes a deal's allocations and inserts them again, so updated_at is when " +
		"the row was written. ProductSetTag hashes it to tell one allocation set from the next.",
	"list_revision": "version copies the list's own version at the revision it records. It is a snapshot " +
		"of the counter on list, not a counter of this row.",
})

func lifecycleTables(t *testing.T) []string {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if m := lifecycleColumn.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			seen[m[1]] = true
		}
	}
	tables := make([]string, 0, len(seen))
	for table := range seen {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

// updatedTables collects every table a production statement updates.
func updatedTables(t *testing.T) map[string]bool {
	t.Helper()
	updated := map[string]bool{}
	packageConstants := map[string]map[string]string{}
	note := func(sql string) {
		for _, m := range updateTarget.FindAllStringSubmatch(sql, -1) {
			updated[strings.ToLower(m[1])] = true
		}
		for _, m := range upsertTarget.FindAllStringSubmatch(sql, -1) {
			updated[strings.ToLower(m[1])] = true
		}
	}
	for _, root := range []string{"internal", "pkg", "../extensions"} {
		err := gatekit.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch {
			case d.IsDir() && d.Name() == "testdata":
				return filepath.SkipDir
			case d.IsDir():
				return nil
			case strings.HasSuffix(path, ".sql") && !strings.HasSuffix(path, ".down.sql"):
				return noteFunctionBodies(path, note)
			case !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
				return nil
			}
			src, err := gatekit.ReadFile(path)
			if err != nil {
				return err
			}
			for _, sql := range gatekit.SQLStatementsIn(t, path, string(src)) {
				note(sql)
			}
			noteRowWritingCalls(t, path, updated, packageConstants)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	for _, dir := range []string{"migrations/baseline/core", "migrations/core"} {
		entries, err := gatekit.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".up.sql") {
				if err := noteFunctionBodies(filepath.Join(dir, entry.Name()), note); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return updated
}

func noteFunctionBodies(path string, note func(string)) error {
	raw, err := gatekit.ReadFile(path)
	if err != nil {
		return err
	}
	for _, m := range sqlFunction.FindAllStringSubmatch(string(raw), -1) {
		note(m[2])
	}
	return nil
}

// noteRowWritingCalls records the table a storekit row writer is handed, as a
// literal or as a constant the package declares.
func noteRowWritingCalls(t *testing.T, path string, updated map[string]bool, packageConstants map[string]map[string]string) {
	t.Helper()
	file, err := gatekit.ParseFile(path, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	constants := packageStringConstants(t, filepath.Dir(path), packageConstants)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		case *ast.Ident:
			name = fn.Name
		}
		at, ok := rowWritingCalls[name]
		if !ok || len(call.Args) <= at {
			return true
		}
		switch arg := call.Args[at].(type) {
		case *ast.BasicLit:
			if table, err := strconv.Unquote(arg.Value); err == nil {
				updated[table] = true
			}
		case *ast.Ident:
			if table, ok := constants[arg.Name]; ok {
				updated[table] = true
			}
		}
		return true
	})
}

func packageStringConstants(t *testing.T, dir string, packageConstants map[string]map[string]string) map[string]string {
	t.Helper()
	if cached, ok := packageConstants[dir]; ok {
		return cached
	}
	constants := map[string]string{}
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(dir, entry.Name()), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					if lit, ok := value.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if text, err := strconv.Unquote(lit.Value); err == nil {
							constants[name.Name] = text
						}
					}
				}
			}
		}
	}
	packageConstants[dir] = constants
	return constants
}

func TestALifecycleColumnSitsOnARowThatIsUpdated(t *testing.T) {
	t.Parallel()
	defer appendOnlyLifecycle.AssertAllMatched(t)
	tables := lifecycleTables(t)
	if len(tables) < lifecycleTableFloor {
		t.Fatalf("the catalog yields %d tables with a lifecycle column, under the floor of %d; "+
			"the reader has stopped matching the catalog's line shape", len(tables), lifecycleTableFloor)
	}
	updated := updatedTables(t)
	for _, table := range tables {
		if updated[table] || appendOnlyLifecycle.Waived(t, table) {
			continue
		}
		t.Errorf("%s declares version or updated_at, and no production statement updates its rows. "+
			"Drop the column in a migration, or waive the table in appendOnlyLifecycle with why it stays.", table)
	}
}
