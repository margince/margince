// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What the nightly repair finds, and — the part that matters — what it stops
// finding.
//
// A sweep that keeps re-offering the same contact is worse than one that misses
// them: it never drains, every tick rewrites the same rows, and the pass looks
// stuck rather than done. So the selector asks its question with the SAME guards
// as the write, and the case that proves it is a merge — because the link lands
// under the survivor while the participant row can still name the source, and a
// selector comparing the raw id would find that row again forever.
//
// This shape cannot be produced by the current writers: the merge now repoints
// participant rows, so the row has to be built by hand, exactly as a database
// that predates that fix holds it.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheSweepFindsTheContactsOwedARepairAndThenStops(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	const email = "owed@sweep.test"
	res, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, email, "Owed Contact", "sweep.test"))
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	// Mail captured while nobody knew the sender — the ordering the whole repair
	// exists for.
	stranded := e.seedCapturedMail(ctx, t, email, "from")

	owed, err := e.store.PeopleOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatalf("listing the owed: %v", err)
	}
	if !containsPerson(owed, res.PersonID) {
		t.Fatalf("the sweep does not see %s, whose captured mail is not on their record", res.PersonID)
	}

	if _, err := e.store.RepairPersonCohort(ctx, res.PersonID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if linked, named := e.cohortStateOf(ctx, t, stranded, res.PersonID); !linked || !named {
		t.Fatalf("after the repair: linked=%v named=%v, want both", linked, named)
	}

	// And now nothing. A pass that keeps finding the same work never drains.
	after, err := e.store.PeopleOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatalf("listing the owed again: %v", err)
	}
	if containsPerson(after, res.PersonID) {
		t.Error("the sweep still offers a contact it just repaired: the selection does not drain, so every tick rewrites the same rows")
	}
}

// A participant row still naming a merged-away person is the shape that used to
// loop: the repair writes the link under the SURVIVOR, so a selector comparing
// the source's own id finds the same row on every pass, forever.
func TestTheSweepDrainsAContactWhoseParticipantRowPredatesAMerge(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	survivor, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "keeps@sweep.test", "Keeps", "sweep.test"))
	if err != nil {
		t.Fatalf("ensure survivor: %v", err)
	}
	retired, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "retires@sweep.test", "Retires", "sweep.test"))
	if err != nil {
		t.Fatalf("ensure retired: %v", err)
	}
	stranded := e.seedCapturedMail(ctx, t, "retires@sweep.test", "from")

	// The pre-merge shape, built by hand, because no current writer produces it:
	// the source redirects, and BOTH its address and its participant rows stay
	// on the source. That is what a database written before the merge repointed
	// participants actually holds, and it is the shape that loops — the repair
	// writes the link under the survivor while the selector, if it compared the
	// raw id, would keep finding the source's own row.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE person SET merged_into_id = $2 WHERE id = $1`,
			retired.PersonID, survivor.PersonID); err != nil {
			return err
		}
		// The address follows the survivor, as a merge moves it.
		if _, err := tx.Exec(ctx,
			`UPDATE person_email SET person_id = $2, is_primary = false WHERE person_id = $1`,
			retired.PersonID, survivor.PersonID); err != nil {
			return err
		}
		// The participant row does NOT: it stays on the source, which is the
		// half the merge used to leave behind.
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (id, activity_id, person_id, role)
			VALUES ($1, $2, $3, 'cc')`, ids.NewV7(), stranded, retired.PersonID)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	owed, err := e.store.PeopleOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatalf("listing the owed: %v", err)
	}
	// The SURVIVOR is offered, never the retired record: a repair run against a
	// record no read returns writes rows nobody opens.
	if containsPerson(owed, retired.PersonID) {
		t.Error("the sweep offers a merged-away record; a repair under it lands on a page nobody can open")
	}
	if !containsPerson(owed, survivor.PersonID) {
		t.Fatalf("the sweep does not offer the survivor %s, who now owns the stranded mail", survivor.PersonID)
	}

	if _, err := e.store.RepairPersonCohort(ctx, survivor.PersonID); err != nil {
		t.Fatalf("repair: %v", err)
	}
	if linked, _ := e.cohortStateOf(ctx, t, stranded, survivor.PersonID); !linked {
		t.Fatalf("the stranded mail is not on the survivor's record")
	}

	after, err := e.store.PeopleOwedACohortRepair(ctx, 50)
	if err != nil {
		t.Fatalf("listing the owed again: %v", err)
	}
	if containsPerson(after, survivor.PersonID) {
		t.Error("the sweep still offers the survivor after repairing them — the merge case is exactly where a selector that " +
			"compares the raw id loops forever, and a job that never drains reads as stuck rather than done")
	}
}

// containsPerson reports whether the sweep offered this contact.
func containsPerson(people []ids.PersonID, want ids.PersonID) bool {
	for _, p := range people {
		if p == want {
			return true
		}
	}
	return false
}
