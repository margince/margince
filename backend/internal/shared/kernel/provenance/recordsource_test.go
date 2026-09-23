// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"slices"
	"testing"
)

func TestTheRetiredSpellingsAreSortedAndHoldNoDuplicate(t *testing.T) {
	t.Parallel()
	retired := RetiredRecordSourceSpellings()
	if len(retired) == 0 {
		t.Fatal("an empty retired set makes every census over it pass having judged nothing")
	}
	if !slices.IsSorted(retired) {
		t.Errorf("the retired spellings are reported unsorted (%v), so a gate's findings reorder between runs", retired)
	}
	if compacted := slices.Compact(slices.Clone(retired)); len(compacted) != len(retired) {
		t.Errorf("a spelling is listed twice in %v, which reports one site as two findings", retired)
	}
}

func TestTheCanonicalWordIsNotAlsoRetired(t *testing.T) {
	t.Parallel()
	if slices.Contains(RetiredRecordSourceSpellings(), RecordSourceManual) {
		t.Errorf("%q is listed as retired, so the census would refuse the very spelling it exists to require", RecordSourceManual)
	}
}

func TestTheRetiredSetIsACopyTheCallerCannotWriteThrough(t *testing.T) {
	t.Parallel()
	first := RetiredRecordSourceSpellings()
	first[0] = "clobbered"
	if second := RetiredRecordSourceSpellings(); second[0] == "clobbered" {
		t.Error("a caller's write reached the owner's own set, so one gate can silently narrow another's corpus")
	}
}
