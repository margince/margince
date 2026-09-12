// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who a contact works for once the job that answered that question ends.
//
// `is_current_primary` answers "which company does this contact currently work
// for". The three moments a caller STATES something already keep it true — a
// contact's only current employment lands primary, ending one clears the flag,
// and a job already over never takes it. This is the moment nobody states
// anything: the primary edge is retired, and the answer silently becomes
// nobody while the contact is plainly still employed somewhere.
//
// ONE REMAINING EMPLOYMENT IS NOT A CHOICE. It is the only answer there is, so
// writing it invents nothing. Leaving it unset would mean the product knows the
// answer, has no alternative to weigh, and declines to say it.
//
// TWO REMAINING IS A CHOICE, and this does not make it. Picking between two
// employers on somebody's behalf writes a fact nobody asserted, and
// `is_current_primary` is a column humans set deliberately — that property is
// worth keeping exactly where it means something. The contact is left with no
// primary employer, which is a state the reads render as such rather than as
// blank.
//
// Both doors reach this, because either alone leaves the same defect reachable
// through the other: archiving the primary edge, and ending it by patch.
//
// The per-contact write lock lives here too. It is what makes every decision
// about one contact's employment flags happen one at a time, which is the same
// question this file's promotion asks and the reason the two belong together.

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

// promoteLoneSurvivingEmployment gives the current-primary flag to this
// contact's remaining employment when exactly one remains and nothing holds the
// flag.
//
// Called where an employment is RETIRED — archived, or ended by date. Not where
// a caller STATES the flag: somebody sending is_current_primary = false has said
// this is not the contact's primary employer, and answering that by promoting the
// same row back is the product overruling a human with the answer they rejected.
// The empty slot is the instruction there, not a gap.
//
// It asks its own question rather than trusting the caller's: both call sites
// hand it a contact, and every "is there anything to promote" test is in the
// statement. A Go-side second spelling of "is this employment over" would drift
// from the one in the patch, and the pair produce a row whose flag and whose
// dates disagree about the same job.
//
// The two predicates are deliberately different questions, and the kernel says
// why. The candidate must be a CURRENT employment (date-aware: a job somebody
// leaves next month is still theirs, and a job that ended yesterday is not).
// Whether the slot is free is date-BLIND, because that is what
// uq_rel_current_primary_employer asks — a date-aware test here would read the
// slot as free while the index still held it, and the write would 409 rather
// than skip.
//
// The count is taken over the same set the promotion draws from, in one
// statement, so no interleaving can promote a second: the caller holds this
// contact's employment write identity, and the unique index is underneath either
// way.
func promoteLoneSurvivingEmployment(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	if _, err := tx.Exec(ctx, `
		WITH remaining AS (
			SELECT id FROM relationship
			 WHERE contact_id = $1 AND kind = 'employment' AND archived_at IS NULL
			   AND `+employment.IsCurrentSQL("ended_at")+`
		)
		UPDATE relationship SET is_current_primary = true
		 WHERE id = (SELECT id FROM remaining LIMIT 1)
		   AND (SELECT count(*) FROM remaining) = 1
		   AND NOT EXISTS (
			SELECT 1 FROM relationship
			 WHERE contact_id = $1 AND `+employment.CurrentPrimarySlotSQL("")+`)`,
		contactID); err != nil {
		return fmt.Errorf("contacts: promoting the surviving employment for contact %s: %w", contactID, err)
	}
	return nil
}
