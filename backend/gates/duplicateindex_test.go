// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// One set of columns, one index.
//
// Two indexes over the same columns with the same predicate answer the same
// questions. The second is a B-tree written on every insert and update of the
// row, a second set of pages competing for cache, and a line of DDL the next
// reader has to account for before concluding it does nothing. Nobody chooses
// that; it arrives.
//
// Nothing announces it: the schema is valid, every query is answered, and the
// only sign is a write cost nobody attributes to anything.
//
// A unique constraint counts as an index, because Postgres backs every one with
// an index — and counting it is safe here, unlike in
// TestEveryNarrowIndexTwinSaysWhatItBuys. That gate compares a predicated index
// against a wide one, where uniqueness is a constraint the wide one cannot
// enforce; here the columns and predicate are IDENTICAL, so one of the two
// enforces everything the other does.

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// coveringIndex reads an index record down to what decides whether two of them
// are the same index: the table, the column list and the predicate.
var coveringIndex = regexp.MustCompile(
	`^(?:public|ext)\.[a-z0-9_]+ CREATE (?:UNIQUE )?INDEX ([a-z0-9_]+) ON (?:public|ext)\.([a-z0-9_]+) USING btree \((.*)\)(.*)$`)

// constraintIndex reads the UNIQUE and PRIMARY KEY constraints, which Postgres
// backs with an index the catalog also prints separately. Both spellings are
// read so a plain index duplicating a constraint's columns is found — the
// second of the two shapes this exists for.
var constraintIndex = regexp.MustCompile(
	`^(?:public|ext)\.([a-z0-9_]+)\.([a-z0-9_]+) (?:UNIQUE|PRIMARY KEY) \((.*)\)$`)

// duplicateIndexFloor is the smallest number of index sets this gate may read
// and still be believed. A reader that stopped reading finds no duplicates and
// reports clean in the same words as a schema that has none.
const duplicateIndexFloor = 800

func TestOneSetOfColumnsCarriesOneIndex(t *testing.T) {
	t.Parallel()

	type key struct{ table, columns, where string }
	names := map[key]map[string]bool{}
	add := func(at key, name string) {
		if names[at] == nil {
			names[at] = map[string]bool{}
		}
		names[at][name] = true
	}
	for _, record := range catalogRecords(t) {
		record = strings.TrimSpace(record)
		if m := coveringIndex.FindStringSubmatch(record); m != nil {
			add(key{table: m[2], columns: m[3], where: strings.TrimSpace(m[4])}, m[1])
			continue
		}
		if m := constraintIndex.FindStringSubmatch(record); m != nil {
			add(key{table: m[1], columns: m[3]}, m[2])
		}
	}
	if len(names) < duplicateIndexFloor {
		t.Fatalf("read only %d index set(s) and expects at least %d — the catalog reader is broken, "+
			"not the schema", len(names), duplicateIndexFloor)
	}

	for at, covering := range names {
		if len(covering) < 2 {
			continue
		}
		both := make([]string, 0, len(covering))
		for name := range covering {
			both = append(both, name)
		}
		sort.Strings(both)
		predicate := ""
		if at.where != "" {
			predicate = " " + at.where
		}
		t.Errorf("%s(%s)%s carries %d indexes — %s. They answer the same questions, so all but one is "+
			"a B-tree written on every insert and update of the row for nothing. Drop the ones that are "+
			"not a constraint's own index; where both are constraints, keep the one something cites.",
			at.table, at.columns, predicate, len(both), strings.Join(both, ", "))
	}
}
