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
// lockOverrideFamily — so neither ever waits on a subject key while holding a
// family key, and the two classes cannot cycle. Family against family: a revoke
// may hold several family keys and takes them far-to-near along one ancestry; a
// carry reads its roots in one statement under one snapshot, sorted by id.
// Neither can take two in opposite orders.

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
// ship its own event and its own audit row. The contact is the ROW's subject
// and not the caller's: a carried copy belongs to the survivor's stream, which
// is where it was announced. Scanned as a pointer because the schema admits a
// lead-held row (contact_id NULL); no writer produces one today, which
// overridesubject_test.go holds, and a reader that assumed NOT NULL would be
// the first thing to break the day that changes.
type revokedOverride struct {
	id      ids.UUID
	contact *ids.UUID
}

// lockOverrideFamily serialises every writer of one carry chain.
//
// Keyed on the chain's ROOT, which every member can derive. The root is read
// before the key is held and can move when an erasure deletes an ancestor
// (carried_from is ON DELETE SET NULL, so the orphaned copy becomes a root of
// its own); lockOverrideFamilyOf re-derives it under the key until the two
// agree. The walk that follows does not rely on the root it read staying a
// row: revokeOverrideChain anchors on the named row too.
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

// overrideFamilyRoot is the one answer both halves of a revoke start from: the
// family lock is keyed on it, and the walk that takes the rows back starts at
// it, so the set the lock serialises and the set the UPDATE reaches are the
// same set whichever member of the chain the caller named. Zero for an id that
// is not a row; the caller's own read then answers 404, so a bad id learns
// nothing from which of the two spoke.
func overrideFamilyRoot(ctx context.Context, tx pgx.Tx, id ids.UUID) (ids.UUID, error) {
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
		return ids.UUID{}, nil
	}
	if err != nil {
		return ids.UUID{}, fmt.Errorf("consent: finding this override's carry chain: %w", err)
	}
	return root, nil
}

// lockOverrideFamilyOf locks the chain the named row belongs to and answers the
// root it locked, re-reading until the two agree. The root is read before the
// lock is held, and carried_from is ON DELETE SET NULL: an erasure of the
// retired source committing in between deletes the root and re-roots the copy
// under its own id, so the key just taken serialises nothing a later carry
// would take. Reading again under the key, and locking the new root when it
// moved, ends with a key both this revoke and any carry extending the chain
// derive from the same row. The root moves only when an ancestor is deleted,
// always toward the named row, so the loop ends within the chain's depth, and
// the keys are taken far-to-near along one ancestry, so two revokes cannot take
// them in opposite orders. The advisory lock is reentrant and held to commit,
// so a stale key taken on the way stays held harmlessly.
func lockOverrideFamilyOf(ctx context.Context, tx pgx.Tx, id ids.UUID) (ids.UUID, error) {
	root, err := overrideFamilyRoot(ctx, tx, id)
	if err != nil {
		return ids.UUID{}, err
	}
	for {
		if err := lockOverrideFamily(ctx, tx, root); err != nil {
			return ids.UUID{}, err
		}
		again, err := overrideFamilyRoot(ctx, tx, id)
		if err != nil {
			return ids.UUID{}, err
		}
		if again == root {
			return root, nil
		}
		root = again
	}
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

// revokeOverrideChain takes back the whole chain the given ROOT heads — the
// original and every copy a merge made of it — and names each row it took.
//
// Every copy carries the original's authority verbatim (overridecarry.go), so
// the caller's one CanRevoke check answers for the whole chain: a merge cannot
// introduce a descendant recorded at a level the caller could not have revoked.
//
// Anchored on the named row as well as the root. The root was read in an
// earlier statement, and carried_from is ON DELETE SET NULL: an erasure of the
// retired source committing in between deletes the root and orphans the copy,
// so a walk from the root alone would match nothing and the door would answer
// success having taken nothing back. The named row is locked FOR UPDATE and
// live, so starting there as well reaches it whatever happened to its root.
func revokeOverrideChain(ctx context.Context, tx pgx.Tx, root, named ids.UUID) ([]revokedOverride, error) {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE chain AS (
		    SELECT id FROM communication_override WHERE id IN ($1, $2)
		  UNION
		    SELECT carried.id
		      FROM communication_override carried
		      JOIN chain ON carried.carried_from = chain.id
		)
		UPDATE communication_override
		   SET revoked_at = now()
		 WHERE id IN (SELECT id FROM chain) AND revoked_at IS NULL
		RETURNING id, contact_id`, root, named)
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

// revokeNote is what every audit row of one revoke says: the same words on each
// contact the chain reached, differing only in which row that contact lost.
type revokeNote struct {
	recordedAtLevel, revokedByLevel commsauthz.AuthorityLevel
	by, reason                      string
	// rows is how many the one click took back in all — one for a vouch that
	// never travelled, more when a merge had copied it. Repeated on every row
	// so an auditor reading one contact's history sees the click's full reach
	// without joining the others.
	rows int
}

// auditAndEmitRevokedOverrides writes one audit row and one event per row the
// walk took back, each on the contact that row belongs to.
//
// PER ROW, NOT PER CALL. The caller named one record; the walk changed one row
// on each record in the chain, and the write shape is domain row + audit row +
// event per record changed. A single audit on the caller's record would leave
// every survivor's history ending at "override carried", with its copy revoked
// in the table and nothing on that record saying so.
//
// CONTACT ROWS ONLY: public-events.yaml declares consent.override_lifted with a
// static x-entity-type: contact, and the audit's entity is the same contact.
// No writer produces a lead-held row today (overridesubject_test.go); the nil
// skip is the schema's shape, not a path.
func auditAndEmitRevokedOverrides(
	ctx context.Context, tx pgx.Tx, revoked []revokedOverride, note revokeNote,
) error {
	for _, r := range revoked {
		if r.contact == nil {
			continue
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "update", entityContact, *r.contact,
			map[string]any{
				"revoked_override":  r.id.String(),
				"revoked_rows":      note.rows,
				"recorded_at_level": string(note.recordedAtLevel),
				"revoked_by_level":  string(note.revokedByLevel),
				"revoked_by":        note.by,
				fieldReason:         note.reason,
			})
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, *r.contact,
			overrideLiftedPayload(r.id, note.recordedAtLevel, note.revokedByLevel)); err != nil {
			return err
		}
	}
	return nil
}
