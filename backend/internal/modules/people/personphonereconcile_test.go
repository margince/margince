// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The defect this guards: a person legitimately holds one number twice under two
// types, and a save that re-sends both rows unchanged must keep them distinct —
// each submitted row placed onto its OWN held row, not both onto the last.
func TestReconcileKeepsTwoRowsOfOneNumberDistinct(t *testing.T) {
	a, b := ids.UUID{1}, ids.UUID{2}
	held := map[string][]ids.UUID{"+49111": {a, b}}
	submitted := []PersonPhoneInput{
		{Phone: "+49111", PhoneType: "work", Position: 0},
		{Phone: "+49111", PhoneType: "home", Position: 1},
	}

	updates, fresh, archive := reconcilePhonePlacements(held, submitted)

	if len(fresh) != 0 || len(archive) != 0 {
		t.Fatalf("fresh=%v archive=%v, want both empty", fresh, archive)
	}
	if len(updates) != 2 {
		t.Fatalf("updates=%d, want 2", len(updates))
	}
	if updates[0].id != a || updates[1].id != b {
		t.Fatalf("updates hit ids %v,%v, want %v,%v (each held row once)",
			updates[0].id, updates[1].id, a, b)
	}
	if updates[0].row.PhoneType != "work" || updates[1].row.PhoneType != "home" {
		t.Fatalf("placements = %q,%q, want work,home", updates[0].row.PhoneType, updates[1].row.PhoneType)
	}
}

// A number the person no longer lists is archived; a number they did not hold is
// fresh; count reductions (held twice, submitted once) archive the surplus.
func TestReconcileFreshAndArchive(t *testing.T) {
	a, b, c := ids.UUID{1}, ids.UUID{2}, ids.UUID{3}
	held := map[string][]ids.UUID{"+49111": {a, b}, "+49999": {c}}
	submitted := []PersonPhoneInput{
		{Phone: "+49111", PhoneType: "work", Position: 0},   // consumes a; b is surplus
		{Phone: "+49222", PhoneType: "mobile", Position: 1}, // brand new
	}

	updates, fresh, archive := reconcilePhonePlacements(held, submitted)

	if len(updates) != 1 || updates[0].id != a {
		t.Fatalf("updates=%v, want one hitting %v", updates, a)
	}
	if len(fresh) != 1 || fresh[0].Phone != "+49222" {
		t.Fatalf("fresh=%v, want [+49222]", fresh)
	}
	// b (surplus of a re-listed number) and c (a dropped number) both archived.
	got := map[ids.UUID]bool{}
	for _, id := range archive {
		got[id] = true
	}
	if len(archive) != 2 || !got[b] || !got[c] {
		t.Fatalf("archive=%v, want {%v,%v}", archive, b, c)
	}
}
