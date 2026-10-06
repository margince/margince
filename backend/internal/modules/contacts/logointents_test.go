// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"testing"
)

// The mark reference check reads every column the list names, on every table.
//
// The statement is BUILT, so the thing worth testing is that the build covers the
// list: a clause dropped for one table reads as "nobody references this" for every
// mark stored there, and the sweep then deletes the bytes behind a live logo.
func TestTheMarkQueryReadsEveryColumnItIsGiven(t *testing.T) {
	t.Parallel()
	query := unreferencedMarkQuery()
	for table, columns := range LogoKeyColumns {
		if !strings.Contains(query, "FROM "+table+" t") {
			t.Errorf("the query does not read %s at all, so every mark stored there is "+
				"unreferenced to it", table)
		}
		for _, column := range columns {
			if !strings.Contains(query, "t."+column+" = k") {
				t.Errorf("the query does not compare %s.%s, so a mark in that column is "+
					"offered to the reaper", table, column)
			}
		}
	}
	// The shape the caller depends on: one bound parameter, and a NOT EXISTS per
	// table so a key referenced anywhere survives.
	if !strings.Contains(query, "unnest($1::text[])") {
		t.Errorf("the query does not take the key list as one bound parameter: %s", query)
	}
	if got := strings.Count(query, "NOT EXISTS"); got != len(LogoKeyColumns) {
		t.Errorf("the query has %d NOT EXISTS clauses for %d tables: a missing one lets a "+
			"referenced key through as unreferenced", got, len(LogoKeyColumns))
	}
}

// The statement is the same on every call, so a plan cache and a reader see one query.
//
// Map iteration order is deliberately random in Go, and a statement that varies per
// call defeats the cache and makes two readers of one log disagree about what ran.
func TestTheMarkQueryIsStableAcrossCalls(t *testing.T) {
	t.Parallel()
	first := unreferencedMarkQuery()
	for range 20 {
		if again := unreferencedMarkQuery(); again != first {
			t.Fatalf("the built statement varies between calls:\n%s\n%s", first, again)
		}
	}
}
