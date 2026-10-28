// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A lead merge carries what names the retired lead by kind and id.
//
// These rows hold no foreign key to lead, so nothing in the schema stops an
// (entity_type, entity_id) pair from naming a record no read returns. The
// contact and company merges have always moved them. The lead merge did not,
// and the merge preview warns about the commitment it then left behind.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALeadMergeCarriesWhatNamesTheRetiredLeadByKindAndID(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	survivor := e.createLead(ctx, t, "Ida Keller", "ida@keller.test", "Keller AG")
	loser := e.createLead(ctx, t, "Ida Kellar", "", "Keller AG")

	// A commitment and a draft, two of the six shapes the carry moves.
	//
	// They have the clearest reader consequence: a week's plan pointing at
	// nothing, and a half-written mail nobody can find.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		plan := ids.NewV7()
		if _, err := tx.Exec(ctx, `
			INSERT INTO weekly_plan (id, owner_id, local_week_start, captured_by)
			VALUES ($1, $2, '2026-10-26', 'human:test')`, plan, e.rep); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO weekly_plan_commitment (id, plan_id, label, linked_record_type, linked_record_id, captured_by)
			VALUES ($1, $2, 'Call Ida back', 'lead', $3, 'human:test')`,
			ids.NewV7(), plan, loser); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO mail_draft (id, anchor_type, anchor_id, author_id)
			VALUES ($1, 'lead', $2, $3)`, ids.NewV7(), loser, e.rep)
		return err
	}); err != nil {
		t.Fatalf("seeding the rows that name the loser by kind: %v", err)
	}

	if _, err := e.store.MergeLead(ctx, loser, survivor); err != nil {
		t.Fatalf("merging: %v", err)
	}

	var commitmentsOnLoser, commitmentsOnSurvivor, draftsOnLoser, draftsOnSurvivor int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		count := func(q string, id ids.LeadID, into *int) error {
			return tx.QueryRow(ctx, q, id).Scan(into)
		}
		const commitments = `SELECT count(*) FROM weekly_plan_commitment
		                      WHERE linked_record_type = 'lead' AND linked_record_id = $1`
		const drafts = `SELECT count(*) FROM mail_draft WHERE anchor_type = 'lead' AND anchor_id = $1`
		if err := count(commitments, loser, &commitmentsOnLoser); err != nil {
			return err
		}
		if err := count(commitments, survivor, &commitmentsOnSurvivor); err != nil {
			return err
		}
		if err := count(drafts, loser, &draftsOnLoser); err != nil {
			return err
		}
		return count(drafts, survivor, &draftsOnSurvivor)
	}); err != nil {
		t.Fatalf("reading after the merge: %v", err)
	}
	if commitmentsOnLoser != 0 || commitmentsOnSurvivor != 1 {
		t.Errorf("commitments: loser=%d survivor=%d, want the week's plan to follow the live lead",
			commitmentsOnLoser, commitmentsOnSurvivor)
	}
	if draftsOnLoser != 0 || draftsOnSurvivor != 1 {
		t.Errorf("drafts: loser=%d survivor=%d, want the half-written mail to follow the live lead",
			draftsOnLoser, draftsOnSurvivor)
	}
}
