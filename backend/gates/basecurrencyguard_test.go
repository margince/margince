// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every table with an fx_rate_to_base column is on the base-currency lock's
// list.
//
// `fx_rate_to_base` is a record's worth expressed against the base in force
// when it froze, so the base may only change while no such record exists.
// deals.frozenRateTables is the list the guard counts, and the list was wrong
// the first time it was written: it named `deal` and missed `offer`, which
// freezes the same column at send. That is not a mistake a reviewer reliably
// catches, because it looks complete.
//
// So the obligation is derived instead. Every table the migrations give an
// `fx_rate_to_base` column must appear in the guard's list — a new one fails
// here rather than silently widening the hole (CLAUDE.md review-loop rule 2).

import (
	"go/ast"
	"go/token"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const dealsDir = "internal/modules/deals"

func TestTheBaseCurrencyGuardCountsEveryFrozenRate(t *testing.T) {
	t.Parallel()
	schema := tablesWithFrozenRateColumn(t)
	if len(schema) == 0 {
		t.Fatal("no table carries fx_rate_to_base — this test has stopped reading the head catalog it derives from")
	}

	counted := map[string]bool{}
	fset := gatekit.SourceFileSet()
	lit := packageVarCompositeLit(t, parsePackageDir(t, fset, dealsDir), "frozenRateTables")
	for _, elt := range lit.Elts {
		bl, ok := elt.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			t.Fatalf("frozenRateTables holds %T, want string literals the schema can be compared against", elt)
		}
		name, err := strconv.Unquote(bl.Value)
		if err != nil {
			t.Fatalf("unquoting %s: %v", bl.Value, err)
		}
		counted[name] = true
	}

	for _, table := range schema {
		if !counted[table] {
			t.Errorf("%s carries fx_rate_to_base but deals.frozenRateTables does not count it, so a workspace holding only %s rows can still change its base currency and silently restate every one of them — add %q to frozenRateTables in %s/basecurrency_store.go",
				table, table, table, dealsDir)
		}
	}
	for table := range counted {
		if !contains(schema, table) {
			t.Errorf("deals.frozenRateTables counts %s, which has no fx_rate_to_base column in the head catalog — the guard runs a query against a column that is not there, and every base-currency read fails", table)
		}
	}
}

// tablesWithFrozenRateColumn reads the head catalog rather than the live
// schema. So the test needs no database, and fails in the unit lane where a
// wrong answer is cheapest to act on.
func tablesWithFrozenRateColumn(t *testing.T) []string {
	t.Helper()
	return slices.Sorted(maps.Keys(parseHeadCatalog(t).tablesWithColumn("fx_rate_to_base", "numeric")))
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}

// NoArchiveColumn tells storekit to drop the `archived_at IS NULL` predicate.
// That is correct exactly when the table has no such column, and a silent
// widening when it does — the write would then reach retired rows the ordinary
// path refuses. The claim is checkable against the schema, so it is checked
// rather than trusted to the constant's name.
func TestNoArchiveColumnIsOnlyPassedForTablesWithoutOne(t *testing.T) {
	t.Parallel()
	archivable := tablesWithArchivedAt(t)
	// Both halves are derived, so both can go silently empty and leave the
	// comparison vacuously true. `company` is the archivable table this
	// whole guard exists around; if the schema reader cannot see its
	// archived_at, it is reading nothing.
	if !archivable["company"] {
		t.Fatal("the catalog reader found no archived_at on company — it has stopped reading the schema it derives from")
	}
	claimed := tablesPassedNoArchiveColumn(t)
	if len(claimed) == 0 {
		t.Fatal("no call site passes storekit.NoArchiveColumn — this test has stopped reading the code it derives from")
	}
	for table, where := range claimed {
		if archivable[table] {
			t.Errorf("%s passes storekit.NoArchiveColumn for %s, but %s has an archived_at column — that write can reach an archived row the ordinary update path refuses; pass storekit.IncludeArchived and mean it, or LiveOnly",
				where, table, table)
		}
	}
}

// noArchiveColumnCall matches `"<table>", …, storekit.NoArchiveColumn)` across
// the wrapped call sites, so the table name is read from the call rather than
// from a list somebody has to remember to update.
var noArchiveColumnCall = regexp.MustCompile(`"([a-z_][a-z0-9_]*)",[^)]*storekit\.NoArchiveColumn`)

func tablesPassedNoArchiveColumn(t *testing.T) map[string]string {
	t.Helper()
	found := map[string]string{}
	err := gatekit.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		raw, err := gatekit.ReadFile(path) // #nosec G304 G122 -- a *.go file from walking the trusted source tree
		if err != nil {
			return err
		}
		for _, m := range noArchiveColumnCall.FindAllStringSubmatch(string(raw), -1) {
			found[m[1]] = path
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	return found
}

func tablesWithArchivedAt(t *testing.T) map[string]bool {
	t.Helper()
	return parseHeadCatalog(t).tablesWithColumn("archived_at", "timestamp with time zone")
}
