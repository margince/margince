// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A partial index whose columns already carry an unpredicated one says why it
// is worth a second B-tree on every write.
//
// An unpredicated index answers every query a predicated one on the SAME columns
// answers: the planner uses it and rechecks the predicate in the heap. So the
// narrow twin buys a smaller scan for its own predicate and costs a second index
// entry on every insert and update of the table — a trade worth making only when
// the predicate is selective enough to matter, and one nobody is making
// deliberately when the pair arrives by accident.
//
// It arrives by accident easily. The cascade-index migration widened nine
// partials and left four standing beside their new wide twins; two more were
// written twice in the baseline; sixteen on `relationship` were three layers
// deep over the same six foreign keys. None of those was a decision.
//
// EXACT COLUMNS ONLY. A narrow index with extra trailing columns answers
// questions the wide one cannot, which is why the cascade migration added those
// beside rather than folding them in. This gate is about the pairs where the
// wide index is a strict superset of what the narrow one can be asked.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// narrowTwin is `table(columns):index`, which is what a reader needs to find
// both halves of the pair.
type narrowTwin string

// deliberateNarrowTwins are the pairs kept on purpose, each saying what the
// narrow index buys that the wide one does not.
var deliberateNarrowTwins = gatekit.Waive(map[narrowTwin]string{
	"deal_room_thread(document_id):idx_deal_room_thread_document_open": "`state = 'open' AND required_change` is " +
		"genuinely selective rather than an archived_at-style liveness filter: the threads a document is waiting on " +
		"are a small slice of the threads it has ever had, so the narrow index stays small and the wide one would " +
		"scan more heap to answer the same question. Measurement could still retire it; the rule the others were " +
		"dropped under does not reach it",
})

// btreeIndex reads one index record: its name, table, column list and whatever
// follows, which is the WHERE clause when it has one.
var btreeIndex = regexp.MustCompile(
	`^(?:public|ext)\.[a-z0-9_]+ CREATE (UNIQUE )?INDEX ([a-z0-9_]+) ON (?:public|ext)\.([a-z0-9_]+) USING btree \(([^)]*)\)(.*)$`)

// narrowTwinFloor is the smallest number of indexes this gate may read and still
// be believed: a reader that stopped reading finds no pairs and reports clean.
const narrowTwinFloor = 200

func TestEveryNarrowIndexTwinSaysWhatItBuys(t *testing.T) {
	t.Parallel()

	type key struct{ table, columns string }
	unpredicated := map[key]string{}
	partials := map[key][]string{}
	read := 0
	for _, record := range catalogRecords(t) {
		m := btreeIndex.FindStringSubmatch(strings.TrimSpace(record))
		// UNIQUE indexes are excluded: theirs is a constraint rather than a
		// read path, and a wide index cannot enforce what a partial one does.
		if m == nil || m[1] != "" {
			continue
		}
		read++
		at := key{table: m[3], columns: m[4]}
		if strings.HasPrefix(strings.TrimSpace(m[5]), "WHERE") {
			partials[at] = append(partials[at], m[2])
			continue
		}
		unpredicated[at] = m[2]
	}
	if read < narrowTwinFloor {
		t.Fatalf("read only %d non-unique btree index(es) and expects at least %d — "+
			"the catalog reader is broken, not the schema", read, narrowTwinFloor)
	}

	for at, narrow := range partials {
		wide, covered := unpredicated[at]
		if !covered {
			continue
		}
		for _, name := range narrow {
			subject := narrowTwin(at.table + "(" + at.columns + "):" + name)
			if deliberateNarrowTwins.Waived(t, subject) {
				continue
			}
			t.Errorf("%s costs a second index entry on every write of %s, and %s answers every query it "+
				"answers. Drop it, or declare it in deliberateNarrowTwins with what its predicate buys",
				subject, at.table, wide)
		}
	}
	deliberateNarrowTwins.AssertAllMatched(t)
}
