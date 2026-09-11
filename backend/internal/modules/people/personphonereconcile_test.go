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
	work, home := ids.UUID{1}, ids.UUID{2}
	held := map[string][]heldPhone{"+49111": {{work, "work"}, {home, "home"}}}
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
	// Each row reclaims its OWN held row by (number, type), so work stays on the
	// work id and home on the home id — not both rewritten to the last submitted.
	if updates[0].id != work || updates[1].id != home {
		t.Fatalf("updates hit ids %v,%v, want %v,%v (each held row once)",
			updates[0].id, updates[1].id, work, home)
	}
	if updates[0].row.PhoneType != "work" || updates[1].row.PhoneType != "home" {
		t.Fatalf("placements = %q,%q, want work,home", updates[0].row.PhoneType, updates[1].row.PhoneType)
	}
}

// Dropping one of two types of a number must archive the row of the DROPPED type
// and keep the survivor on its own id — the exact case the by-value reconciler
// got wrong, relabelling the work row to "home" and archiving the real home row,
// so the survivor inherited the work row's created_at, source and observed_at.
func TestReconcileDropOneOfTwoTypesKeepsTheRightRow(t *testing.T) {
	work, home := ids.UUID{1}, ids.UUID{2}
	held := map[string][]heldPhone{"+493011112222": {{work, "work"}, {home, "home"}}}
	// The reader kept only the home row.
	submitted := []PersonPhoneInput{{Phone: "+493011112222", PhoneType: "home", Position: 0}}

	updates, fresh, archive := reconcilePhonePlacements(held, submitted)

	if len(fresh) != 0 {
		t.Fatalf("fresh=%v, want empty", fresh)
	}
	if len(updates) != 1 || updates[0].id != home {
		t.Fatalf("updates=%v, want one hitting the HOME id %v (not the work id %v)", updates, home, work)
	}
	if len(archive) != 1 || archive[0] != work {
		t.Fatalf("archive=%v, want [%v] (the dropped work row)", archive, work)
	}
}

// A row whose type is the only change reclaims the SAME held row rather than
// archiving it and inserting a fresh one — there is no same-type row to reclaim,
// so the FIFO fallback lands the retype on the existing id and its history.
func TestReconcileTypeChangeKeepsExistingRow(t *testing.T) {
	existing := ids.UUID{1}
	held := map[string][]heldPhone{"+49111": {{existing, "work"}}}
	submitted := []PersonPhoneInput{{Phone: "+49111", PhoneType: "home", Position: 0}}

	updates, fresh, archive := reconcilePhonePlacements(held, submitted)

	if len(fresh) != 0 || len(archive) != 0 {
		t.Fatalf("fresh=%v archive=%v, want both empty (the row is kept, not replaced)", fresh, archive)
	}
	if len(updates) != 1 || updates[0].id != existing {
		t.Fatalf("updates=%v, want one hitting %v", updates, existing)
	}
	if updates[0].row.PhoneType != "home" {
		t.Fatalf("placement type = %q, want home", updates[0].row.PhoneType)
	}
}

// A number the person no longer lists is archived; a number they did not hold is
// fresh; count reductions (held twice, submitted once) archive the surplus.
func TestReconcileFreshAndArchive(t *testing.T) {
	a, b, c := ids.UUID{1}, ids.UUID{2}, ids.UUID{3}
	held := map[string][]heldPhone{"+49111": {{a, "work"}, {b, "work"}}, "+49999": {{c, "mobile"}}}
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
