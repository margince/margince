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

	// A half-written mail nobody can find is the clearest reader consequence.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
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

	var draftsOnLoser, draftsOnSurvivor int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		count := func(q string, id ids.LeadID, into *int) error {
			return tx.QueryRow(ctx, q, id).Scan(into)
		}
		const drafts = `SELECT count(*) FROM mail_draft WHERE anchor_type = 'lead' AND anchor_id = $1`
		if err := count(drafts, loser, &draftsOnLoser); err != nil {
			return err
		}
		return count(drafts, survivor, &draftsOnSurvivor)
	}); err != nil {
		t.Fatalf("reading after the merge: %v", err)
	}
	if draftsOnLoser != 0 || draftsOnSurvivor != 1 {
		t.Errorf("drafts: loser=%d survivor=%d, want the half-written mail to follow the live lead",
			draftsOnLoser, draftsOnSurvivor)
	}
}
