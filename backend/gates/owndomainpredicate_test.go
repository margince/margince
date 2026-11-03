// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Every own-domain decision compares one folded form of a domain.
//
// owndomain.Fold IDNA-encodes, so anna@bücher.example is at xn--bcher-kva.example.
// Postgres cannot fold IDNA. SQL that takes her domain off the address misses
// the match, while the reply path folds in Go and calls her ours. So writers
// store the folded domain, and SQL compares only those columns.

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// derivesAMailDomain is the shape of taking a domain off an address in SQL.
var derivesAMailDomain = regexp.MustCompile(`'@'|FROM\s+'@`)

// TestEveryOwnDomainComparisonInSQLReadsFoldedDomains fails when SQL takes a
// domain off an address. It also fails when a caller of owndomain.CoversSQL
// passes anything but a folded column.
func TestEveryOwnDomainComparisonInSQLReadsFoldedDomains(t *testing.T) {
	t.Parallel()
	files := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return slices.ContainsFunc(gatekit.SQLStatementsOf(file), derivesAMailDomain.MatchString) ||
				len(coversSQLCalls(file)) > 0
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}.Files(t)

	var derived, unfolded []string
	calls := 0
	for _, source := range files {
		if slices.ContainsFunc(gatekit.SQLStatementsOf(source.File), derivesAMailDomain.MatchString) {
			derived = append(derived, source.Path)
		}
		for _, call := range coversSQLCalls(source.File) {
			calls++
			if column, ok := gatekit.LiteralText(call.Args[0]); !ok || !strings.HasSuffix(column, "_folded") {
				unfolded = append(unfolded, source.Path+": "+gatekit.TextOf(call.Args[0]))
			}
		}
	}
	// Fewer callers than this means the walk lost one, or a caller went back to
	// comparing addresses.
	if calls < 9 {
		t.Errorf("found %d call(s) of owndomain.CoversSQL, want at least the nine that compare folded domains", calls)
	}
	if len(derived) > 0 {
		t.Errorf("these files take a mail domain off an address in SQL: %v\n\n"+
			"Postgres cannot fold IDNA, so the domain compares unequal to the folded form of the same "+
			"domain. Compare a *_folded column its writers fill through owndomain.StoredOfAddress, "+
			"through owndomain.CoversSQL when a subdomain counts, or fold the address in Go.", derived)
	}
	if len(unfolded) > 0 {
		t.Errorf("owndomain.CoversSQL compares a value no writer folded: %v\n\n"+
			"Pass a *_folded column that its writers fill through owndomain.StoredOfAddress or StoredOwn.", unfolded)
	}
}

// coversSQLCalls returns the calls of owndomain.CoversSQL and CoversIndexedSQL in a file.
func coversSQLCalls(file *ast.File) []*ast.CallExpr {
	qualifier, _ := gatekit.ImportedAs(file, "github.com/margince/margince/backend/internal/platform/owndomain")
	if qualifier == "" {
		return nil
	}
	var calls []*ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || len(call.Args) != 2 {
			return true
		}
		if selector, isSelector := call.Fun.(*ast.SelectorExpr); isSelector && (selector.Sel.Name == "CoversSQL" || selector.Sel.Name == "CoversIndexedSQL") {
			if pkg, isIdent := selector.X.(*ast.Ident); isIdent && pkg.Name == qualifier {
				calls = append(calls, call)
			}
		}
		return true
	})
	return calls
}

// foldedColumn is a column the migrations add to hold a folded domain, and the
// column it is folded from.
type foldedColumn struct {
	table, source, folded string
}

var addsAFoldedColumn = regexp.MustCompile(`ALTER TABLE (\w+) ADD COLUMN IF NOT EXISTS (\w+_folded)\b`)

// foldedColumns derives the folded columns from the migrations that add them,
// so a new one is held here without being listed.
func foldedColumns(t *testing.T) []foldedColumn {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(gatekit.ModuleRoot(t), "migrations", "core", "*.up.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("reading the core migrations: %v (found %d)", err, len(paths))
	}
	var columns []foldedColumn
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, match := range addsAFoldedColumn.FindAllStringSubmatch(string(body), -1) {
			source := strings.TrimSuffix(match[2], "_domain_folded")
			if source == match[2] {
				source = strings.TrimSuffix(match[2], "_folded")
			}
			columns = append(columns, foldedColumn{table: match[1], source: source, folded: match[2]})
		}
	}
	return columns
}

// foldedAfterTheStatement ratifies a writer whose address SQL derives, so Go
// folds it in the same transaction right after the statement returns the rows.
var foldedAfterTheStatement = gatekit.Waive(map[string]string{
	"internal/modules/activities/participantbackfill.go: WITH candidate AS ( …": "derives the address from activity.counterparty_email in SQL; foldInsertedParticipants folds the RETURNING rows in the same transaction",
})

// TestEveryWriterOfAFoldedSourceWritesTheFold fails when a production statement
// writes an address or domain without the folded column beside it. Such a row
// is read as an outsider by every own-domain check that compares in SQL.
func TestEveryWriterOfAFoldedSourceWritesTheFold(t *testing.T) {
	t.Parallel()
	columns := foldedColumns(t)
	if len(columns) == 0 {
		t.Fatal("no migration adds a *_folded column, so this census reads nothing")
	}
	writesASource := func(statement string) bool {
		return slices.ContainsFunc(columns, func(column foldedColumn) bool {
			writes, _ := writesColumn(statement, column)
			return writes
		})
	}
	writers := make(map[foldedColumn]int, len(columns))
	var missing []string
	files := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return slices.ContainsFunc(gatekit.SQLStatementsOf(file), writesASource)
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}.Files(t)
	for _, source := range files {
		for _, statement := range gatekit.SQLStatementsOf(source.File) {
			for _, column := range columns {
				writes, folds := writesColumn(statement, column)
				if !writes {
					continue
				}
				writers[column]++
				site := source.Path + ": " + gatekit.FirstLineOf(statement)
				if !folds && !foldedAfterTheStatement.Waived(t, site) {
					missing = append(missing, site)
				}
			}
		}
	}
	foldedAfterTheStatement.AssertAllMatched(t)
	for _, column := range columns {
		if writers[column] == 0 {
			t.Errorf("found no writer of %s.%s, so the census can no longer see the statements it must hold", column.table, column.source)
		}
	}
	if len(missing) > 0 {
		t.Errorf("these statements write an address or domain without its folded column:\n\t%s\n\n"+
			"Write the folded column in the same statement, through owndomain.StoredOfAddress or StoredOwn.",
			strings.Join(missing, "\n\t"))
	}
}

// writesColumn reports whether a statement writes column.source on its table,
// and whether it writes column.folded too. It reads the statement through
// sqlWrites, the tree's one reading of a write's table and columns.
func writesColumn(statement string, column foldedColumn) (writes, folds bool) {
	for _, target := range sqlWrites(statement) {
		if target.table == column.table && slices.Contains(target.cols, column.source) {
			writes, folds = true, slices.Contains(target.cols, column.folded)
		}
	}
	return writes, folds
}

// TestEveryFoldedColumnIsStampedByAPatch holds storekit.FoldedDomains, the list
// a Patch write stamps from, to the folded columns the migrations add. A column
// missing from it is one an edit through a Patch leaves stale.
func TestEveryFoldedColumnIsStampedByAPatch(t *testing.T) {
	t.Parallel()
	var migrated, stamped []string
	for _, column := range foldedColumns(t) {
		migrated = append(migrated, column.table+"."+column.source+" -> "+column.folded)
	}
	for _, column := range storekit.FoldedDomains() {
		stamped = append(stamped, column.Table+"."+column.Source+" -> "+column.Folded)
	}
	slices.Sort(migrated)
	slices.Sort(stamped)
	if !slices.Equal(migrated, stamped) {
		t.Errorf("storekit.FoldedDomains lists\n\t%s\nand the migrations add\n\t%s\n"+
			"List every folded column there, with the fold its writers store.",
			strings.Join(stamped, "\n\t"), strings.Join(migrated, "\n\t"))
	}
}
