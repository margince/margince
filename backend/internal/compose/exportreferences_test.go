// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A member may export fewer columns than the table holds — exportableColumns
// reads the live catalog, and a reference the page does not carry has nothing
// to withhold. Reaching the visibility probe here would mean asking about a
// column that is not in the answer, on a transaction the caller has every right
// not to have opened.
func TestAPageWithoutTheReferenceColumnAsksNothing(t *testing.T) {
	rows := [][]any{{ids.NewV7(), "Reisser"}}
	// A nil transaction is the assertion: the probe is what would use it, and
	// the columns say there is nothing to probe.
	if err := withholdUnreadableReferences(t.Context(), nil, tableDeal, []string{"id", "name"}, rows); err != nil {
		t.Fatalf("a page carrying no reference column: %v", err)
	}
	if len(rows[0]) != 2 || rows[0][1] != "Reisser" {
		t.Fatalf("the page was altered: %v", rows)
	}
}

// A table that references nothing row-scoped is answered before any column is
// resolved, which is what keeps the withholding off the tables it has no
// question about.
func TestATableWithNoReferencesAsksNothing(t *testing.T) {
	if err := withholdUnreadableReferences(t.Context(), nil, "product", []string{"id", "sku"}, [][]any{{ids.NewV7(), "A-1"}}); err != nil {
		t.Fatalf("a table with no declared references: %v", err)
	}
}

func TestIndexOfColumnAnswersMinusOneForAColumnThePageLacks(t *testing.T) {
	if at := indexOfColumn([]string{"id", "name"}, refColCompany); at != -1 {
		t.Fatalf("a column the page lacks resolved to %d", at)
	}
	if at := indexOfColumn([]string{"id", refColCompany}, refColCompany); at != 1 {
		t.Fatalf("the reference column resolved to %d", at)
	}
}

// pgx hands a uuid column back as [16]byte, and the same value can arrive
// already typed when a row was assembled in Go. Both are the same reference,
// and reading only one of them would leave the other unprobed — a silent pass
// rather than a failure, because an unrecognised value looks exactly like a
// row that names nothing.
func TestAReferenceIsReadFromEitherSpellingOfAUUID(t *testing.T) {
	id := ids.NewV7()
	for _, c := range []struct {
		name string
		row  []any
	}{
		{"as pgx returns it", []any{[16]byte(id)}},
		{"already typed", []any{id}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, named := referencedID(c.row, 0)
			if !named || got != id {
				t.Fatalf("read %v (named=%v), want %v", got, named, id)
			}
		})
	}
}

// Anything else names nothing. A masked column arrives as a value this pair
// never declared, and a row shorter than the index is a page that does not
// carry the column at all — neither is an error, and treating either as a
// reference would probe a value that is not one.
func TestAValueThatIsNotAReferenceNamesNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		row  []any
		at   int
	}{
		{"a masked column", []any{"withheld"}, 0},
		{"an absent value", []any{nil}, 0},
		{"past the end of the row", []any{}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, named := referencedID(c.row, c.at); named {
				t.Fatalf("%s was read as a reference", c.name)
			}
		})
	}
}
