// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockContactForEmployment serializes every writer of one contact's employment
// flags, which is what the demote-then-grant pair below needs to be one unit.
// Without it two patches on DIFFERENT employments of the same contact each read
// "no primary elsewhere" and each grant the flag, and the second commit answers
// 409 on uq_rel_current_primary_employer — the same race the create path already
// closed, reached through the other verb.
//
// Silent for anything that is not an employment: a deal stakeholder or a partner
// edge shares none of this state, and taking a contact lock for one would
// serialize writes that never contend.
func lockContactForEmployment(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var contactID *ids.ContactID
	err := tx.QueryRow(ctx,
		`SELECT contact_id FROM relationship WHERE id = $1 AND kind = 'employment'`, id).Scan(&contactID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Not an employment, or gone. Either way the row lock below is what
		// reports it, and it reports it the same way it always has.
		return nil
	case err != nil:
		return fmt.Errorf("contacts: reading the employment's contact for the write lock: %w", err)
	case contactID == nil:
		return nil
	}
	return storekit.LockWriteIdentity(ctx, tx, employmentKind, contactID.String())
}

// Retiring an employment fills an empty primary slot only when no choice remains.
// Explicitly clearing the primary flag does not invoke this succession rule.
func promoteLoneSurvivingEmployment(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	if err := flipCurrentPrimary(ctx, tx, `UPDATE relationship SET is_current_primary = true
 WHERE NOT is_current_primary AND id = (`+employmentSuccessorSQL()+`)`, pgx.StrictNamedArgs{contactEntity: contactID, "excluded": nil}); err != nil {
		return fmt.Errorf("contacts: promoting the surviving employment for contact %s: %w", contactID, err)
	}
	return nil
}

// flipCurrentPrimary runs an UPDATE that moves only is_current_primary on edges
// beside the one being written. Each edge it moves gets an audit row and an
// event, like any other edge update. The statement must match only rows holding
// the opposite value, which is what makes the before image exact.
func flipCurrentPrimary(ctx context.Context, tx pgx.Tx, update string, args ...any) error {
	rows, err := tx.Query(ctx, update+" RETURNING "+relationshipColumns, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	flipped, err := scanRelationships(rows)
	if err != nil {
		return err
	}
	for _, rel := range flipped {
		before := relationshipFieldImage(rel)
		before["is_current_primary"] = !rel.IsCurrentPrimary
		if err := emitRelationshipChange(ctx, tx, "update", before, rel); err != nil {
			return err
		}
	}
	return nil
}

// The candidate is date-aware; slot occupancy follows the date-blind unique index.
// Excluding the proposed archive lets preview and execution share the same choice.
func employmentSuccessorSQL() string {
	return `WITH remaining AS (
 SELECT id FROM relationship WHERE contact_id = @contact AND kind = 'employment' AND archived_at IS NULL
 AND (@excluded::uuid IS NULL OR id <> @excluded) AND ` + employment.IsCurrentSQL("ended_at") + `
 ) SELECT id FROM remaining WHERE (SELECT count(*) FROM remaining) = 1
 AND NOT EXISTS (SELECT 1 FROM relationship WHERE contact_id = @contact
 AND (@excluded::uuid IS NULL OR id <> @excluded) AND ` + employment.CurrentPrimarySlotSQL("") + `)`
}
