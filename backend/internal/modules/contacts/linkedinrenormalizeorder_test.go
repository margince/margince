// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The groups that write are ordered, and the ones that do not are not locked at all.
//
// The order IS the lock order: groups are disjoint, so two passes requesting them in
// one sequence wait on each other rather than deadlock, and an import's narrower set
// is that sequence with gaps. A group already one row at its current key would hold a
// row for the rest of the sweep's transaction to prove there was nothing to do.
func TestOnlyTheGroupsThatWriteAreLockedAndTheyAreOrdered(t *testing.T) {
	t.Parallel()
	low, mid, high := idAt(t, "11"), idAt(t, "55"), idAt(t, "99")
	atKey, needsRekey := idAt(t, "22"), idAt(t, "77")

	wanted := map[ids.UUID]string{
		low: "najahak.io", mid: "najahak.io", high: "qubix",
		atKey: "settled", needsRekey: "moved",
	}
	rowAt := func(id ids.UUID, key string) ghostKeyRow {
		return ghostKeyRow{id: id, stored: &key}
	}
	groups := map[string][]ghostKeyRow{
		"later":    {rowAt(high, "qubix")},                      // one row, key already right
		"pair":     {rowAt(mid, "najahak.io"), rowAt(low, "x")}, // a fold
		"settled":  {rowAt(atKey, "settled")},                   // nothing to do
		"rekeyOne": {rowAt(needsRekey, "stale")},                // a re-key
	}

	work := workInLockOrder(groups, wanted)
	if len(work) != 2 {
		t.Fatalf("locking %d groups, want 2: a group already at its key writes nothing, and a "+
			"lock taken to prove that blocks every import behind the sweep", len(work))
	}
	if got := lowestID(work[0]); got != low {
		t.Errorf("the first group locks %s, want the lowest id %s: map order gave no order at "+
			"all, which is how two passes take one pair in opposite sequence", got, low)
	}
	if got := lowestID(work[1]); got != needsRekey {
		t.Errorf("the second group locks %s, want %s", got, needsRekey)
	}
}

// idAt builds a uuid whose ordering is legible in a failure message.
func idAt(t *testing.T, prefix string) ids.UUID {
	t.Helper()
	id, err := ids.Parse(prefix + "000000-0000-4000-8000-000000000000")
	if err != nil {
		t.Fatalf("building a test id: %v", err)
	}
	return id
}

// Names that differ only cosmetically derive one key, and an absent name derives none.
//
// This equivalence is what lets the sweep re-read a group under its lock and still
// recognise it: if these spellings produced different keys, a re-import that only
// restated the employer would read as a row that left its group, and the duplicate
// would stand on every pass.
func TestCosmeticSpellingsOfOneEmployerDeriveOneKey(t *testing.T) {
	t.Parallel()
	plain := currentKey(ptr("najahak.io"))
	if plain == "" {
		t.Fatal("a named employer derived no key, so every row with a name groups as though it " +
			"had none")
	}

	for _, spelling := range []string{"  najahak.io  ", "NAJAHAK.IO", "najahak.io | Growth"} {
		if got := currentKey(ptr(spelling)); got != plain {
			t.Errorf("%q derives %q, want %q: a row restating its employer would read as one "+
				"that left its group", spelling, got, plain)
		}
	}

	if got := currentKey(nil); got != "" {
		t.Errorf("an absent name derives %q, want the empty key it groups under", got)
	}
}
