// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A row keyed by a client's request id is written conflict-first.
//
// The unique index refuses a repeated delivery; the ON CONFLICT clause is what turns
// that refusal into the replay the caller is owed. An insert that carries the key and
// omits the clause reads as protected and answers a double-click with a 500 — and
// only when two deliveries overlap, which is not a shape a reviewer sees in a diff.
//
// The corpus comes from the SCHEMA rather than a list here: whichever tables carry a
// request-key index are the tables this covers, so adding one to a new table enrols
// its writers without touching this file.
//
// What this holds and nothing else does: an insert carrying a request key has a
// conflict clause at all. That clause infers on the index's own columns. A statement
// with no ON CONFLICT is valid SQL against a partial index, and it raises 23505 on
// the second delivery. No check of a clause's shape can see that. Whether the clause
// repeats the index's predicate is asked of every upsert by
// TestAnUpsertOntoAPartialIndexRepeatsItsPredicate.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	insertStatement = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(\w+)\s*\(([^)]*)\)`)
	onConflict      = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\(([^)]*)\)\s*(?:WHERE\s+[^)]*?)?\s*DO\s+NOTHING`)
)

// tablesKeyedByRequest picks every partial unique index keyed by request_id from
// the head catalog. It reads through the predicate gate's reader, so the two gates
// cannot see different indexes.
func tablesKeyedByRequest(t *testing.T) map[string]uniqueIndex {
	t.Helper()
	keyed := map[string]uniqueIndex{}
	for table, indexes := range uniqueIndexesByTable(t) {
		for _, index := range indexes {
			if index.predicate != "" && strings.Contains(index.columns, "request_id") {
				keyed[table] = index
			}
		}
	}
	if len(keyed) == 0 {
		t.Fatal("no partial request-key index found in the head catalog — this gate reads the " +
			"schema for its subject, so an empty corpus means the reading has stopped matching " +
			"rather than that nothing needs covering")
	}
	return keyed
}

func TestARequestKeyedInsertIsWrittenConflictFirst(t *testing.T) {
	t.Parallel()
	keyed := tablesKeyedByRequest(t)
	root := gatekit.ModuleRoot(t)
	written := map[string]int{}
	for _, path := range goSourceFiles(t, filepath.Join(root, "internal")) {
		// Production writers only. A test may write without the clause on purpose —
		// the control beside publicroomreplay's overlap case does exactly that, to
		// hold that the index is still there refusing a repeat. Covering tests here
		// would make that control unwritable, and it is the thing that proves the
		// clause is load-bearing rather than decorative.
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		// statementsIn rather than a per-literal reader: `"INSERT … " + clause` is
		// ONE statement to Postgres and several literals to the parser, and no
		// literal alone holds both halves this compares. Per literal, the gate
		// passes silently over the form somebody reaches for when a line gets long.
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("relative path of %s: %v", path, err)
		}
		for _, statement := range statementsIn(file) {
			insert := insertStatement.FindStringSubmatch(statement)
			if insert == nil {
				continue
			}
			want, covered := keyed[insert[1]]
			// Folded, like every pattern around it: a column list spelling the key
			// REQUEST_ID would otherwise register the table and skip the check,
			// which is the one direction this must not fail in.
			if !covered || !strings.Contains(strings.ToLower(insert[2]), "request_id") {
				continue
			}
			written[insert[1]]++
			assertConflictFirst(t, rel, insert[1], want, statement)
		}
	}
	// PER TABLE, not a total. A count alone passes when one table carries two
	// writers and another carries none — so the index on the second is protecting
	// nothing and the census says so by staying quiet, which is the one way it must
	// not break.
	for table, index := range keyed {
		if written[table] == 0 {
			t.Errorf("%s is keyed by %s and no writer in the tree inserts into it with a "+
				"request_id: either the index protects nothing, or this gate has stopped "+
				"seeing the statement that writes it", table, index.name)
		}
	}
}

func assertConflictFirst(t *testing.T, where, table string, want uniqueIndex, statement string) {
	t.Helper()
	clause := onConflict.FindStringSubmatch(statement)
	if clause == nil {
		t.Errorf("%s: an insert into %s carries request_id and no ON CONFLICT … DO NOTHING. "+
			"%s refuses the second delivery of one attempt, so without the clause a "+
			"double-click raises 23505 and the caller is told its own write failed",
			where, table, want.name)
		return
	}
	if got := flattenColumns(clause[1]); got != want.columns {
		t.Errorf("%s: the insert into %s infers on (%s) and %s is on (%s). Inference is "+
			"by expression, so a mismatch is not refused at build time — it raises 42P10 "+
			"on the first replay", where, table, got, want.name, want.columns)
	}
}
