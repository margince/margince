// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A plain index does not hold a prefix of another index's columns.
//
// A B-tree on (a, b) answers every lookup on (a), so a narrow index beside it is
// read by nothing the wide one cannot serve, and costs a second index entry on
// every insert and update of the row. Fifteen such pairs stood.
//
// Three exclusions, each one a way the plain rule takes something load-bearing:
//
// A PRIMARY KEY prefixes a wider unique on six tables here. It is not a read
// path — it is the table's identity and what its inbound foreign keys reference.
// An index BACKING a unique constraint goes with the constraint if dropped, so
// it is never the redundant half. And a PARTIAL index covers nothing outside its
// predicate, so it cannot stand in for a plain one.
//
// That last one is why the predicate is parsed rather than pattern-matched off
// the end of the definition: a `NULLS NOT DISTINCT` between the column list and
// the WHERE made two partial indexes read as unpredicated, and the pair they
// then appeared to cover was a cascade index on a column an erasure deletes
// through. The migration lane's own cascade gate caught it. A census that mis-
// reads its subject reports confidently, which is the only way this one fails.

import (
	"regexp"
	"strings"
	"testing"
)

// btreeDefinition splits an index record into its name, table and the text from
// the column list onward. The columns and the predicate are taken apart by
// balanced parentheses rather than by a regex, because a column list can hold
// one — `lower(email)`, `(a || b)` — and the modifiers between the list and the
// WHERE are not fixed.
var btreeDefinition = regexp.MustCompile(
	`^(?:public|ext)\.([A-Za-z0-9_]+) CREATE (UNIQUE )?INDEX \S+ ON (?:public|ext)\.([a-z0-9_]+) USING btree (\(.*)$`)

// constraintDefinition names an index Postgres built for a constraint. Dropping
// one drops the constraint, so it is never the redundant half of a pair.
var constraintDefinition = regexp.MustCompile(
	`^(?:public|ext)\.([a-z0-9_]+)\.([a-z0-9_]+) (?:UNIQUE(?: NULLS NOT DISTINCT)?|PRIMARY KEY) \(`)

// prefixIndexFloor is the smallest number of btree indexes this gate may read
// and still be believed: a reader that stopped parsing finds no pairs and
// reports the same clean schema as one that has none.
const prefixIndexFloor = 800

type btreeIndexDefinition struct {
	table, name string
	// unique is read because a UNIQUE index enforces something a wider
	// non-unique one does not. It is never the redundant half of a pair, however
	// much of its column list the wider index repeats.
	unique  bool
	columns []string
	// tail is everything after the column list: modifiers and the predicate. Two
	// indexes are comparable only when it matches, which is what keeps a partial
	// index from standing in for a plain one.
	tail string
}

func TestNoPlainIndexIsAPrefixOfAnother(t *testing.T) {
	t.Parallel()
	backsAConstraint := map[string]bool{}
	var indexes []btreeIndexDefinition
	for _, record := range catalogRecords(t) {
		record = strings.TrimSpace(record)
		if m := constraintDefinition.FindStringSubmatch(record); m != nil {
			backsAConstraint[m[2]] = true
			continue
		}
		m := btreeDefinition.FindStringSubmatch(record)
		if m == nil {
			continue
		}
		columns, tail, ok := splitColumnList(m[4])
		if !ok {
			t.Fatalf("%s: the column list of %q does not close; the parse this census depends on is broken",
				m[3], m[1])
		}
		indexes = append(indexes, btreeIndexDefinition{
			table: m[3], name: m[1], unique: m[2] != "", columns: columns, tail: tail,
		})
	}
	if len(indexes) < prefixIndexFloor {
		t.Fatalf("parsed only %d btree index(es) and expects at least %d — the catalog reader is broken, "+
			"not the schema", len(indexes), prefixIndexFloor)
	}

	for _, narrow := range indexes {
		if !canBeRedundant(narrow, backsAConstraint) {
			continue
		}
		for _, wide := range indexes {
			if !coversAsPrefix(narrow, wide) {
				continue
			}
			t.Errorf("%s(%s) is a prefix of %s(%s) on %s, so every lookup it answers is one %s answers "+
				"too — and it is a second index entry on every insert and update. Drop it, or say here "+
				"what it buys.",
				narrow.name, strings.Join(narrow.columns, ", "), wide.name,
				strings.Join(wide.columns, ", "), narrow.table, wide.name)
			break
		}
	}
}

// canBeRedundant reports whether narrow is the kind of index that CAN be the
// redundant half of a pair.
//
// A unique index never is, whether or not a constraint declared it: the
// uniqueness is the point, and a wider non-unique sibling does not hold it. An
// exact-duplicate pair of uniques is TestOneSetOfColumnsCarriesOneIndex's
// finding, not this one's. An index backing a constraint goes with the
// constraint if dropped.
func canBeRedundant(narrow btreeIndexDefinition, backsAConstraint map[string]bool) bool {
	return !narrow.unique && !backsAConstraint[narrow.name]
}

// coversAsPrefix reports whether wide answers everything narrow does: same
// table, same predicate and modifiers, and narrow's columns a strict prefix of
// wide's.
func coversAsPrefix(narrow, wide btreeIndexDefinition) bool {
	if narrow.name == wide.name || narrow.table != wide.table || narrow.tail != wide.tail {
		return false
	}
	return isPrefix(narrow.columns, wide.columns)
}

// splitColumnList takes the balanced column list off the front of an index
// definition and answers it with whatever follows.
//
// Only TOP-LEVEL commas separate columns. An expression column carries its own —
// `coalesce(a, b)` is one column, not two — and splitting on every comma would
// compare two indexes by column lists neither of them has.
func splitColumnList(definition string) (columns []string, tail string, ok bool) {
	depth, start := 0, 1
	for i, ch := range definition {
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return append(columns, strings.TrimSpace(definition[start:i])),
					strings.TrimSpace(definition[i+1:]), true
			}
		case ',':
			if depth == 1 {
				columns = append(columns, strings.TrimSpace(definition[start:i]))
				start = i + 1
			}
		}
	}
	return nil, "", false
}

// isPrefix reports whether narrow is a STRICT prefix of wide. Strict, because
// equal column sets are TestOneSetOfColumnsCarriesOneIndex's finding and would
// otherwise be reported twice.
func isPrefix(narrow, wide []string) bool {
	if len(narrow) >= len(wide) {
		return false
	}
	for i, column := range narrow {
		if wide[i] != column {
			return false
		}
	}
	return true
}

// The exclusions and the parse, driven by cases that MUST fire.
//
// The census above only asserts absence, so a parse that quietly stopped seeing
// its subject would report a clean schema — and the floor cannot tell that apart,
// because it counts indexes rather than decisions.
func TestThePrefixRuleKnowsWhatItMayNotDrop(t *testing.T) {
	t.Parallel()
	plain := func(name string, unique bool, columns ...string) btreeIndexDefinition {
		return btreeIndexDefinition{table: "t", name: name, unique: unique, columns: columns}
	}
	wide := plain("wide", false, "a", "b")

	if !coversAsPrefix(plain("narrow", false, "a"), wide) {
		t.Error("a plain (a) beside a plain (a, b) was not recognised, so the census finds nothing to report")
	}
	if !canBeRedundant(plain("narrow", false, "a"), map[string]bool{}) {
		t.Error("a plain index was excluded, so every finding this gate exists for is skipped")
	}
	if canBeRedundant(plain("narrow", true, "a"), map[string]bool{}) {
		t.Error("a UNIQUE (a) was reported as droppable beside a non-unique (a, b) — the wider index does " +
			"not hold the uniqueness, so dropping it loses a guarantee")
	}
	if canBeRedundant(plain("narrow", false, "a"), map[string]bool{"narrow": true}) {
		t.Error("an index backing a constraint was reported as droppable, and dropping it drops the constraint")
	}
	if coversAsPrefix(plain("narrow", false, "b"), wide) {
		t.Error("(b) was read as a prefix of (a, b); only a LEADING match is served by the wider index")
	}

	partial := wide
	partial.tail = "WHERE (archived_at IS NULL)"
	if coversAsPrefix(plain("narrow", false, "a"), partial) {
		t.Error("a partial index was read as covering a plain one, which is how a cascade index gets dropped")
	}

	for _, tc := range []struct {
		name       string
		definition string
		columns    []string
		tail       string
	}{
		{"a plain list", "(a, b) WHERE x", []string{"a", "b"}, "WHERE x"},
		{"an expression carrying its own comma", "(coalesce(a, b), c)", []string{"coalesce(a, b)", "c"}, ""},
		{"one expression column", "(lower(email))", []string{"lower(email)"}, ""},
		{"modifiers before the predicate", "(a) NULLS NOT DISTINCT WHERE (y)", []string{"a"}, "NULLS NOT DISTINCT WHERE (y)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			columns, tail, ok := splitColumnList(tc.definition)
			if !ok {
				t.Fatalf("splitColumnList(%q) did not close", tc.definition)
			}
			if strings.Join(columns, "|") != strings.Join(tc.columns, "|") {
				t.Errorf("columns = %q, want %q", columns, tc.columns)
			}
			if tail != tc.tail {
				t.Errorf("tail = %q, want %q", tail, tc.tail)
			}
		})
	}
	if _, _, ok := splitColumnList("(a, b"); ok {
		t.Error("an unclosed column list parsed, so a malformed record would be compared as if it were read")
	}
}
