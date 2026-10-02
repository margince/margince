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
// What this holds and nothing else does: that an insert carrying a request key has a
// conflict clause AT ALL. A statement with no ON CONFLICT is valid SQL against a
// partial index — it simply raises 23505 on the second delivery — so no check of a
// clause's shape can see it, only a check that asks why a keyed insert has none.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	// CREATE UNIQUE INDEX … ON <table> (room_id, request_id) WHERE <predicate>
	requestKeyIndex = regexp.MustCompile(
		`(?is)CREATE\s+UNIQUE\s+INDEX\s+(\w+)\s+ON\s+(\w+)\s*\(([^)]*request_id[^)]*)\)\s*WHERE\s+([^;]+);`)
	insertStatement = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(\w+)\s*\(([^)]*)\)`)
	onConflict      = regexp.MustCompile(`(?is)ON\s+CONFLICT\s*\(([^)]*)\)\s*(?:WHERE\s+([^)]*?))?\s*DO\s+NOTHING`)
)

type requestKeyedTable struct {
	index     string
	columns   string
	predicate string
}

// tablesKeyedByRequest reads the migrations for every request-key index there is.
func tablesKeyedByRequest(t *testing.T) map[string]requestKeyedTable {
	t.Helper()
	root := moduleRoot(t)
	keyed := map[string]requestKeyedTable{}
	migrations, err := filepath.Glob(filepath.Join(root, "migrations", "core", "*.up.sql"))
	if err != nil {
		t.Fatalf("listing the migrations: %v", err)
	}
	for _, path := range migrations {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, found := range requestKeyIndex.FindAllStringSubmatch(string(body), -1) {
			keyed[found[2]] = requestKeyedTable{
				index:     found[1],
				columns:   normaliseList(found[3]),
				predicate: strings.TrimSpace(found[4]),
			}
		}
	}
	if len(keyed) == 0 {
		t.Fatal("no request-key index found in the migrations — this gate reads the schema " +
			"for its subject, so an empty corpus means the pattern below has stopped matching " +
			"rather than that nothing needs covering")
	}
	return keyed
}

func normaliseList(columns string) string {
	parts := strings.Split(columns, ",")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return strings.Join(parts, ", ")
}

func TestARequestKeyedInsertIsWrittenConflictFirst(t *testing.T) {
	t.Parallel()
	keyed := tablesKeyedByRequest(t)
	root := moduleRoot(t)
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
				"seeing the statement that writes it", table, index.index)
		}
	}
}

func assertConflictFirst(t *testing.T, where, table string, want requestKeyedTable, statement string) {
	t.Helper()
	clause := onConflict.FindStringSubmatch(statement)
	if clause == nil {
		t.Errorf("%s: an insert into %s carries request_id and no ON CONFLICT … DO NOTHING. "+
			"%s refuses the second delivery of one attempt, so without the clause a "+
			"double-click raises 23505 and the caller is told its own write failed",
			where, table, want.index)
		return
	}
	if got := normaliseList(clause[1]); got != want.columns {
		t.Errorf("%s: the insert into %s infers on (%s) and %s is on (%s). Inference is "+
			"by expression, so a mismatch is not refused at build time — it raises 42P10 "+
			"on the first replay", where, table, got, want.index, want.columns)
	}
	// A partial index is matched only by a statement that repeats its predicate.
	// Omit it and the clause infers nothing, which is the same 42P10 at runtime.
	if got := strings.TrimSpace(clause[2]); !sameCondition(got, want.predicate) {
		t.Errorf("%s: the insert into %s guards its conflict with %q and %s is partial on %q. "+
			"Postgres matches a partial index only when the statement repeats its predicate",
			where, table, got, want.index, want.predicate)
	}
}

// sameCondition compares two SQL conditions ignoring spacing and case.
func sameCondition(a, b string) bool {
	flatten := func(s string) string {
		return strings.Join(strings.Fields(strings.ToUpper(s)), " ")
	}
	return flatten(a) == flatten(b)
}
