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
// ## Why the set is read from the classification
//
// The classification lives in an integration-tagged file under
// backend/migrations, which this lane cannot import, so it is read as text. A
// copy kept here drifted one way: a new PENDING WRITER entry over there was
// never held. TestEveryPendingWriterEntryIsRead fails if an entry is written in
// a shape the reader cannot parse.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// pendingWriterClassification holds the FK classification. It is
// integration-tagged, so this lane reads its text rather than importing it.
const pendingWriterClassification = "migrations/schema_fitness_integration_test.go"

// pendingWriterEntry is one "table.column" key classified PENDING WRITER.
var pendingWriterEntry = regexp.MustCompile(`"(\w+)\.(\w+)":\s*"PENDING WRITER`)

// pendingWriterTables maps each table with a PENDING WRITER entry to the
// columns that carry the claim, read from the classification itself.
func pendingWriterTables(t *testing.T) (map[string][]string, string) {
	t.Helper()
	source, err := gatekit.ReadFile(pendingWriterClassification)
	if err != nil {
		t.Fatalf("reading %s: %v", pendingWriterClassification, err)
	}
	tables := map[string][]string{}
	for _, entry := range pendingWriterEntry.FindAllStringSubmatch(string(source), -1) {
		tables[entry[1]] = append(tables[entry[1]], entry[2])
	}
	return tables, string(source)
}

func TestNoPendingWriterHasAWriter(t *testing.T) {
	t.Parallel()
	writes := collectTableWrites(t)
	pending, _ := pendingWriterTables(t)

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
			for _, column := range pending[w.table] {
				// An INSERT whose column list this reader could not extract is
				// unknown, not empty. It is reported, so that no claim survives
				// a statement nobody could read.
				if len(w.cols) == 0 {
					landed = append(landed, w.table+"."+column+
						" — an INSERT whose columns could not be read, "+owner+" at "+w.pos)
					continue
				}
				if slices.Contains(w.cols, column) {
					landed = append(landed, w.table+"."+column+" — "+owner+" at "+w.pos)
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
			"Replace it with the gate that covers the write.", finding)
	}
}

// The entries are found by one pattern, so an entry written in a shape it does
// not parse would drop out of the census unseen. Every PENDING WRITER value in
// the classification has to be one the pattern read.
func TestEveryPendingWriterEntryIsRead(t *testing.T) {
	t.Parallel()
	tables, text := pendingWriterTables(t)
	read := 0
	for _, columns := range tables {
		read += len(columns)
	}
	if written := strings.Count(text, `"PENDING WRITER`); written != read {
		t.Errorf("%s holds %d PENDING WRITER entries and this gate read %d — write each key as "+
			"\"table.column\" so the claim is held", pendingWriterClassification, written, read)
	}
}
