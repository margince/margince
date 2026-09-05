// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A "no writer yet" classification stops being true the moment a writer lands.
//
// backend/migrations/schema_fitness_integration_test.go classifies every FK
// column into a row-scoped table, and some entries say PENDING WRITER: nobody
// creates a row here, so there is no gate to name, and the entry carries the
// obligation on whoever writes the first one instead.
//
// Nothing failed when that writer arrived. The entry simply became false, and
// the next reader greps, finds a confident sentence about how the column is
// gated, and stops looking — the failure AGENTS.md's one-source-of-truth page
// and rule 8 both name. It is not hypothetical: five of the seven entries this
// gate was filed over have since gained writers, and each was corrected by
// hand, whenever somebody happened to notice.
//
// So the claim holds itself. A table named here that gains an INSERT fails,
// and the entry has to be replaced with the gate that now covers it.
//
// ## The column, not the table
//
// A table gaining a row creator is not the same event. conversation_claim is
// INSERTed into today and names nine columns, none of them task_activity_id —
// so that claim is still true, and a table-level trigger would have reported it
// false on the day this gate was written. The column list is what the claim is
// about, so the column list is what is read.
//
// ## Why the set is declared here
//
// The classification lives in an integration-tagged file under
// backend/migrations, which this lane cannot import. Rather than reach across,
// each lane holds what it can actually see: the entries there cite this gate,
// and this names the tables. TestEveryPendingWriterTableIsStillClassified is
// what keeps the two from drifting apart — it fails if a table here has no
// PENDING WRITER entry left over there.

import (
	"os"
	"strings"
	"testing"
)

// pendingWriterTables are the tables whose FK classification says no writer
// exists yet. One entry per table, with the column that carries the claim.
// gatekit:fixture the column each table's PENDING WRITER claim is about — a
// subject and its column, not a subject and the cost of exempting it
var pendingWriterTables = map[string]string{
	// conversation_claim IS inserted into — nine columns, none of them this
	// one. The claim is about the column, which is why a table-level trigger
	// would have reported it false on the day this gate was written.
	"conversation_claim": "task_activity_id",
}

func TestNoPendingWriterHasAWriter(t *testing.T) {
	t.Parallel()
	writes := collectTableWrites(t)

	var landed []string
	inserts, withColumns := 0, 0
	for owner, ws := range writes {
		for _, w := range ws {
			if w.verb != "insert" {
				continue
			}
			inserts++
			if len(w.cols) > 0 {
				withColumns++
			}
			column, pending := pendingWriterTables[w.table]
			if !pending {
				continue
			}
			// An INSERT whose column list this reader could not extract is
			// UNKNOWN, not empty — reported, because the alternative is a
			// claim quietly surviving a statement nobody could read.
			if len(w.cols) == 0 {
				landed = append(landed, w.table+"."+column+
					" — an INSERT whose columns could not be read, "+owner+" at "+w.pos)
				continue
			}
			for _, col := range w.cols {
				if col == column {
					landed = append(landed, w.table+"."+column+" — "+owner+" at "+w.pos)
					break
				}
			}
		}
	}

	// Under-recognition is the one way this must not break, and it has two
	// shapes here: a walk that reads no INSERTs at all, and one that reads
	// them but extracts no column lists. Either reports every claim intact.
	if inserts < 100 {
		t.Fatalf("the walk found %d INSERT statements, want at least 100 — "+
			"it is reading less than it thinks, and a census that can fail "+
			"short has already failed", inserts)
	}
	if withColumns*2 < inserts {
		t.Fatalf("only %d of %d INSERTs yielded a column list — the extraction "+
			"is reading less than it thinks, and every claim below rests on it",
			withColumns, inserts)
	}

	for _, finding := range landed {
		t.Errorf("%s is classified PENDING WRITER and now has one.\n"+
			"\tThe entry in backend/migrations/schema_fitness_integration_test.go "+
			"states an obligation on whoever writes the first row. Somebody has. "+
			"Replace it with the gate that covers the write, and drop the table "+
			"from pendingWriterTables here.", finding)
	}
}

// The two lanes hold two halves of one claim, so each has to be able to fail
// when the other moves. This is the half that catches an entry retired over
// there while the table stays listed here — which would leave this gate
// guarding a claim nobody makes any more.
func TestEveryPendingWriterTableIsStillClassified(t *testing.T) {
	t.Parallel()
	const classification = "migrations/schema_fitness_integration_test.go"
	source, err := os.ReadFile(classification)
	if err != nil {
		t.Fatalf("reading %s: %v", classification, err)
	}
	text := string(source)
	if !strings.Contains(text, "PENDING WRITER") {
		t.Fatalf("%s names no PENDING WRITER at all — either every entry was "+
			"retired, in which case pendingWriterTables should be empty, or "+
			"this gate is reading the wrong file", classification)
	}
	for table, column := range pendingWriterTables {
		entry := `"` + table + "." + column + `":`
		if !strings.Contains(text, entry) {
			t.Errorf("%s.%s is listed here but %s carries no entry for it",
				table, column, classification)
			continue
		}
		// The entry exists; it has to still be the PENDING WRITER one. A
		// classification rewritten to name a real gate is the good outcome,
		// and it is the moment this table stops belonging here.
		rest := text[strings.Index(text, entry)+len(entry):]
		line := rest
		if end := strings.Index(rest, "\n"); end >= 0 {
			line = rest[:end]
		}
		if !strings.Contains(line, "PENDING WRITER") {
			t.Errorf("%s.%s is no longer classified PENDING WRITER — drop it "+
				"from pendingWriterTables, which exists to hold that claim",
				table, column)
		}
	}
}
