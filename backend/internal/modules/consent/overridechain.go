// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Taking back a vouch takes back every copy a merge made of it — and says so.
//
// A merge does not move an override: it COPIES it onto the survivor under a new
// id, links the copy back with carried_from and leaves the original live as
// evidence about the record whose rep made it (overridecarry.go). The caller
// holds the id the door gave them, which after a merge is the original. A
// revoke that took back only that row would answer 204 while the survivor's
// copy went on allowing the send — the same vouch, still standing, under an id
// its author was never told about.
//
// The stop direction survives the identical shape because it fails SAFE: a
// stale lift leaves the survivor suppressed. This one fails OPEN, which is why
// the walk lives here and not in lift.go — and why the two things a walk alone
// does NOT buy are worth naming, because each fails open in its own way:
//
//   - A CHAIN THAT GROWS UNDER THE WALK. The recursive term reads the
//     statement's snapshot. A merge from the survivor, committing a further
//     copy after that snapshot is taken, leaves a live descendant beneath a
//     revoked ancestor — and the subject lock cannot see it coming, because a
//     chain outlives the subject it started on and that merge locks two
//     subjects the revoker never named. lockOverrideFamily closes it by giving
//     both writers one key they can each derive: the chain's root.
//   - A ROW REVOKED IN THE TABLE AND LIVE TO A SUBSCRIBER. Each carried copy
//     ships its own consent.override_recorded naming the NEW row's id
//     (overridecarry.go turns on that being so), so a lifted event naming only
//     the id the caller typed leaves every other stream holding state nothing
//     ever takes back. Hence one event per row, on the stream that heard it
//     recorded.
//
// LOCK ORDER, and it is the whole of the deadlock argument: SUBJECT keys first,
// then FAMILY keys. Both writers hold to it — CarryOverridesTx takes
// lockBothSidesOfACarry before it reaches lockCarriedFamilies, and
// revokeOverrideAdmittedTx takes lockSubjectSuppressions before
// lockOverrideFamilyOf — so neither ever waits on a subject key while holding a
// family key, and the two classes cannot cycle.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// revokedOverride is one row the walk took back, read off RETURNING so each can
// ship its own event. The contact is the ROW's subject and not the caller's: a
// carried copy belongs to the survivor's stream, which is where it was
// announced, and nil for a copy a promotion left on a lead.
type revokedOverride struct {
	id      ids.UUID
	contact *ids.UUID
}

// lockOverrideFamily serialises every writer of one carry chain.
//
// Keyed on the chain's ROOT, which every member can derive and which never
// moves: carried_from is written once at the copy and never updated, so the
// walk upward is over rows already settled. Erasure can cut the link — the
// reference is ON DELETE SET NULL — and that is harmless here: the orphaned
// copy simply becomes a root of its own, and the rows that would have shared
// its key are gone.
//
// Transaction-scoped and spelled like every other lock in this package, over a
// key no subject can collide with.
func lockOverrideFamily(ctx context.Context, tx pgx.Tx, root ids.UUID) error {
	if root.IsZero() {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"communication_override_family:"+root.String()); err != nil {
		return fmt.Errorf("consent: serialising writes to this override's carry chain: %w", err)
	}
	return nil
}

// lockOverrideFamilyOf is the revoke side: the chain of the one row the caller
// named. A row that is not there locks nothing and leaves the caller's own
// read to answer 404, so a bad id learns nothing from which of the two spoke.
func lockOverrideFamilyOf(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var root ids.UUID
	err := tx.QueryRow(ctx, `
		WITH RECURSIVE ancestry AS (
		    SELECT id, carried_from FROM communication_override WHERE id = $1
		  UNION ALL
		    SELECT parent.id, parent.carried_from
		      FROM communication_override parent
		      JOIN ancestry ON ancestry.carried_from = parent.id
		)
		SELECT id FROM ancestry WHERE carried_from IS NULL`, id).Scan(&root)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("consent: finding this override's carry chain: %w", err)
	}
	return lockOverrideFamily(ctx, tx, root)
}

// lockCarriedFamilies is the carry side: every chain the copy about to run
// would extend.
//
// Read under the subject locks the carry already holds, which is what makes one
// pass enough — no new source row can appear between this read and the copy,
// because both doors that could write one (Allow, and a carry INTO this
// subject) queue behind those same keys. A row can only be REVOKED meanwhile,
// and the copy re-reads revoked_at at its own snapshot and skips it.
func lockCarriedFamilies(ctx context.Context, tx pgx.Tx, from commsauthz.StopSubject) error {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE ancestry AS (
		    SELECT id, carried_from FROM communication_override
		     WHERE (($1::uuid IS NOT NULL AND contact_id = $1)
		         OR ($2::uuid IS NOT NULL AND lead_id = $2))
		       AND revoked_at IS NULL
		  UNION
		    SELECT parent.id, parent.carried_from
		      FROM communication_override parent
		      JOIN ancestry ON ancestry.carried_from = parent.id
		)
		SELECT id FROM ancestry WHERE carried_from IS NULL ORDER BY id`,
		zeroAsNull(from.ContactID.UUID), zeroAsNull(from.LeadID.UUID))
	if err != nil {
		return fmt.Errorf("consent: finding the carry chains this merge extends: %w", err)
	}
	defer rows.Close()
	// ORDERED BY the query, not by arrival: two carries reaching an overlapping
	// set of chains must take the shared keys in one order, the same property
	// lockSubjectsInOrder sorts for.
	var roots []ids.UUID
	for rows.Next() {
		var root ids.UUID
		if err := rows.Scan(&root); err != nil {
			return fmt.Errorf("consent: reading the carry chains this merge extends: %w", err)
		}
		roots = append(roots, root)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("consent: finding the carry chains this merge extends: %w", err)
	}
	for _, root := range roots {
		if err := lockOverrideFamily(ctx, tx, root); err != nil {
			return err
		}
	}
	return nil
}

// revokeOverrideChain takes back the named row and every copy a merge made of
// it, and names each one it took.
//
// Every copy carries the original's authority verbatim (overridecarry.go), so
// the caller's one CanRevoke check answers for the whole chain: a merge cannot
// introduce a descendant recorded at a level the caller could not have revoked.
func revokeOverrideChain(ctx context.Context, tx pgx.Tx, id ids.UUID) ([]revokedOverride, error) {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE chain AS (
		    SELECT id FROM communication_override WHERE id = $1
		  UNION ALL
		    SELECT carried.id
		      FROM communication_override carried
		      JOIN chain ON carried.carried_from = chain.id
		)
		UPDATE communication_override
		   SET revoked_at = now()
		 WHERE id IN (SELECT id FROM chain) AND revoked_at IS NULL
		RETURNING id, contact_id`, id)
	if err != nil {
		return nil, fmt.Errorf("consent: revoking the override: %w", err)
	}
	defer rows.Close()
	var revoked []revokedOverride
	for rows.Next() {
		var r revokedOverride
		if err := rows.Scan(&r.id, &r.contact); err != nil {
			return nil, fmt.Errorf("consent: reading the revoked overrides: %w", err)
		}
		revoked = append(revoked, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consent: revoking the override: %w", err)
	}
	return revoked, nil
}

// emitOverrideLifted ships one consent.override_lifted per row taken back, each
// on its own subject's stream.
//
// CONTACT ROWS ONLY, the same exclusion auditAndEmitCarriedOverrides makes and
// for the same reason: public-events.yaml declares consent.override_lifted with
// a static x-entity-type: contact, so a lead-held copy would ship an envelope
// naming contact:<lead uuid>, an id of the wrong kind. A promotion is the only
// way to hold one, and it leaves the contact copy — the one a send evaluates —
// announced correctly.
func emitOverrideLifted(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID,
	revoked []revokedOverride, recordedAtLevel, by commsauthz.AuthorityLevel,
) error {
	for _, r := range revoked {
		if r.contact == nil {
			continue
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, *r.contact,
			overrideLiftedPayload(r.id, recordedAtLevel, by)); err != nil {
			return err
		}
	}
	return nil
}
