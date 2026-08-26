// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// A column an update path cannot clear is declared where the reversal path
// reads it.
//
// A module that writes `col = coalesce($n, col)` cannot set that column to NULL
// through the update path: the placeholder's NULL selects the current value. A
// restore of a before-image holding NULL for such a column would report success
// and change nothing — a dishonest success, which is worse than a refusal
// because the human reads the confirmation and stops looking. The reversal path
// refuses those fields instead, and compose.CoalesceGuardedColumns is the set it
// refuses.
//
// That set is a claim about SQL in another package, so this gate holds it equal
// to the SQL rather than trusting it. The claim is the kind that fails short in
// silence: a column added to a coalesce list nobody updated here would leave the
// reversal path reporting success over a field it did not write, and there is no
// assertion anywhere that would notice.
//
// The Scope sweep is what makes "these are all of them" checkable. A gate that
// walked only the six record modules would find exactly the coalesce guards
// those modules hold and report PASS, saying nothing about the ones elsewhere.
// Sweeping the negative space means a guard outside the roots is either found or
// ratified by name.

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

// Record types the reversal path serves. Their tables are named for them, which
// is what lets this gate read a column out of an UPDATE statement and know whose
// it is; TestEveryReversibleRecordTypeHasATableNamedForIt holds that.
var reversibleRecordTypes = []string{
	"person", "organization", "deal", "lead", "project", "activity",
}

// updateSetClause captures the SET list of an UPDATE, up to WHERE or the end of
// the literal. The column list is what carries the guards; a coalesce in a WHERE
// predicate is a read, not a write that cannot clear.
var updateSetClause = regexp.MustCompile(`(?is)UPDATE\s+([a-z_]+)\s+SET\b(.*?)(?:\bWHERE\b|$)`)

// coalesceGuard matches `coalesce($n, col)` — the shape where the placeholder's
// NULL selects the column's current value. `coalesce($n, 'literal')` and
// `coalesce(col, $n)` are different writes and are deliberately not matched:
// the first can be written NULL by nobody, the second takes the placeholder
// only when the column is already NULL.
var coalesceGuard = regexp.MustCompile(`(?i)coalesce\(\s*\$\d+\s*,\s*([a-z_]+)\s*\)`)

// guardsIn reports the (table, column) guards a source file's SQL literals hold.
func guardsIn(file *ast.File) map[string][]string {
	found := map[string][]string{}
	ast.Inspect(file, func(node ast.Node) bool {
		expression, isExpression := node.(ast.Expr)
		if !isExpression {
			return true
		}
		text, isLiteral := gatekit.LiteralText(expression)
		if !isLiteral {
			return true
		}
		for _, statement := range updateSetClause.FindAllStringSubmatch(text, -1) {
			table, setList := statement[1], statement[2]
			for _, guard := range coalesceGuard.FindAllStringSubmatch(setList, -1) {
				found[table] = append(found[table], guard[1])
			}
		}
		return true
	})
	return found
}

// guardedTablesThatAreNotReversibleRecords: every OTHER table under
// internal/modules whose update path holds a coalesce guard. Keyed by table
// rather than by file, because the question is about the table's meaning and a
// table written from two files would otherwise need two identical reasons.
//
// This exists so that "the reversal path serves six record types" is a
// statement somebody checked rather than a silent skip. Without it the census
// would walk past every guard it did not recognise and report PASS, which is a
// census failing short in exactly the way that leaves no assertion to notice.
var guardedTablesThatAreNotReversibleRecords = gatekit.Waive(map[string]string{
	"data_subject_request": "a request's own lifecycle row, not a CRM record; its history and its reversal belong to the DSR workflow rather than to the record spine",
	"voice_corpus_source":  "a corpus membership row; putting one back is re-including the source, which the corpus surface does directly rather than by replaying a field image",
	"voice_profile":        "carries a generated artifact reference derived from a corpus; replaying the prior reference names a build a later generation may have superseded, which is not the same as producing it",
	"relationship":         "an EDGE, and an edge's ends are what it IS; the reversal path serves records, and the edge tier is outside it by design rather than by omission",
	"transcript_read":      "a per-reader position marker, not a field of the activity; it carries no audit images and no history screen shows it",
	"automation":           "a rule definition whose surface is the automation editor; a rule put back by replaying columns would not re-derive the schedule the rule owns",
})

// The declared set is the SQL's set. Both directions matter: a declared column
// the SQL no longer guards makes the reversal path refuse a field it could
// write, and a guarded column nobody declared makes it claim a success it did
// not have.
func TestEveryCoalesceGuardedColumnIsDeclaredWhereTheReversalPathReadsIt(t *testing.T) {
	scope := gatekit.Scope{
		Roots: []string{"internal/modules"},
		Subject: func(path string, file *ast.File) bool {
			return !strings.HasSuffix(path, "_test.go") && len(guardsIn(file)) > 0
		},
	}

	reversible := map[string]bool{}
	for _, recordType := range reversibleRecordTypes {
		reversible[recordType] = true
	}

	found := map[string]map[string]bool{}
	sawAGuard := false
	for _, parsed := range scope.Files(t) {
		for table, columns := range guardsIn(parsed.File) {
			sawAGuard = true
			if !reversible[table] {
				// Named as not-a-record, or reported here. A table nobody
				// ratified is a guard nobody looked at.
				if !guardedTablesThatAreNotReversibleRecords.Waived(t, table) {
					t.Errorf("%s holds a coalesce guard and is neither a record type "+
						"the reversal path serves nor a table anybody ratified as not "+
						"being one", table)
				}
				continue
			}
			if found[table] == nil {
				found[table] = map[string]bool{}
			}
			for _, column := range columns {
				found[table][column] = true
			}
		}
	}
	if !sawAGuard {
		t.Fatal("the census found no coalesce guard anywhere under internal/modules; " +
			"the scan is broken, not the tree — activities/lifecycle.go writes several")
	}
	// A ratification that no longer matches anything reads as approval of code
	// that is gone, which is how a waiver outlives the reason it was written.
	guardedTablesThatAreNotReversibleRecords.AssertAllMatched(t)

	for _, recordType := range reversibleRecordTypes {
		declared := compose.CoalesceGuardedColumns(recordType)
		actual := sortedKeys(found[recordType])
		if strings.Join(declared, ",") == strings.Join(actual, ",") {
			continue
		}
		t.Errorf("%s: the reversal path declares %v as columns it cannot clear, "+
			"but the module's UPDATE statements guard %v.\n"+
			"\tUpdate compose.coalesceGuardedColumns to match the SQL. A column the SQL "+
			"guards and this set omits is a field a restore would report success over "+
			"without writing.", recordType, declared, actual)
	}
}

// The tables are named for the record types, which is what lets a column read
// out of an UPDATE statement be attributed to a record type at all. A rename
// that broke the identity would make every guard on that table invisible to the
// gate above — a census failing short in exactly the silent way.
func TestEveryReversibleRecordTypeHasATableNamedForIt(t *testing.T) {
	schema := readCoreMigrations(t)
	for _, recordType := range reversibleRecordTypes {
		if !regexp.MustCompile(`(?i)CREATE TABLE (IF NOT EXISTS )?` + recordType + `\s*\(`).MatchString(schema) {
			t.Errorf("no table named %q in the baseline migration; the coalesce census "+
				"attributes a guarded column to a record type by the table's name, and "+
				"a table named otherwise is invisible to it", recordType)
		}
	}
}

// readCoreMigrations concatenates the applied core schema. It reads the whole
// directory rather than the baseline alone: a table introduced by a later
// migration is as real as one in the baseline, and a gate that looked only at
// 0001 would report a missing table that exists.
func readCoreMigrations(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join("migrations", "core"))
	if err != nil {
		t.Fatalf("read core migrations: %v", err)
	}
	var schema strings.Builder
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join("migrations", "core", entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		schema.Write(body)
	}
	if schema.Len() == 0 {
		t.Fatal("no core migrations read; the gate would report every table missing")
	}
	return schema.String()
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
