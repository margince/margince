// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Carrying a retiring subject's standing overrides onto the record that
// survives them — CarryStopsTx's sibling (stopcarry.go), split into its own
// file so each carry's own worked-through reasoning stays legible on its own
// rather than doubling one already-long file.
//
// A standing override (override.go's Allow) is a rep vouching that a
// machine-level refusal may be overruled for one category. Left uncarried at
// a merge, it is exactly as quiet a loss as an uncarried stop: the merge
// moves everything else that points at the retiring record, the override
// stays attached to an id the send engine no longer evaluates, and the next
// send to the survivor for that category is refused again — with nothing
// saying a rep already vouched for it.
//
// COPY, NEVER MOVE, for the same three reasons stopcarry.go gives: the
// predecessor's row is evidence that THIS record's rep vouched, it may be
// read in an export or after an un-merge, and a later erasure reaching the
// predecessor must still find its own rows.
//
// NO SEPARATE LOCK METHOD. lockSubjectSuppressions (suppressionlock.go) — the
// lock both Allow and RevokeOverride take — keys on the subject id alone,
// naming no table, and StopCarrier.LockStopsTx already takes that same key on
// both sides of the merge before CarryOverridesTx runs (mergeContactTx calls
// LockStopsTx before it locks any contact row, then CarryStopsTx, then this).
// A dedicated LockOverridesTx would serialise against nothing that lock does
// not already cover.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// CarryOverridesTx implements contacts.StopCarrier.
//
// IDEMPOTENT ON CATEGORY, gated on authority exactly as CarryStopsTx is: a
// survivor already holding a live override for a category keeps the STRONGER
// of its own and the retiring subject's, never a second row of the same
// category — two live rows would leave a rep's reason and an unrelated rep's
// reason both on file for a decision that only ever had one live answer.
//
// THE AUTHORITY — AND THE REASON — TRAVEL. RevokeOverride only lets a caller
// take back a row recorded at a level they outrank. If a merge downgraded a
// carried row to the merging rep's own level, a plain rep could revoke an
// admin's vouch simply by merging the vouched-for contact into one of their
// own first — the same laundering path stopcarry.go closes for a stop. So the
// carried row keeps the original decided_by_level and the original reason,
// verbatim, and gains only a new captured_by (whoever merged) and
// carried_from (which row it came from).
func (s *Store) CarryOverridesTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	// GATED HERE TOO, exactly as CarryStopsTx is and for the same reason: this
	// is exported and takes a transaction it does not own, so anything holding
	// a consent store could call it directly, and the point of the gate is
	// only to keep a caller with NO merging grant at all from writing overrides
	// through a seam meant for merges.
	if err := admitAMergingCaller(ctx); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	moved, err := insertCarriedOverrides(ctx, tx, from, to, by)
	if err != nil {
		return err
	}
	if len(moved) == 0 {
		// Nothing to carry is the ordinary case — most contacts hold no
		// override — and it is not worth an audit row.
		return nil
	}
	return auditAndEmitCarriedOverrides(ctx, tx, to, moved)
}

// carriedOverride is one row the carry moved, read back off RETURNING so each
// can ship its own event.
type carriedOverride struct {
	category string
	level    string
}

// insertCarriedOverrides runs the copy itself. DISTINCT ON collapses several
// live source rows of one category to the strongest, for the reason
// CarryStopsTx's identical comment gives: a subquery in an INSERT ... SELECT
// does not see the rows that statement is inserting, so without this a
// subject holding two live overrides of one category would carry two copies.
// The NOT EXISTS compares authority, not merely category, so a survivor's own
// weaker row of the same category cannot keep a stronger one out.
func insertCarriedOverrides(
	ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject, by string,
) ([]carriedOverride, error) {
	rows, err := tx.Query(ctx, `
		INSERT INTO communication_override
		  (contact_id, lead_id, category, reason, decided_by_level, captured_by, carried_from)
		SELECT DISTINCT ON (live.category)
		       $3, $4, live.category, live.reason, live.decided_by_level, $5, live.id
		  FROM communication_override live
		 WHERE (($1::uuid IS NOT NULL AND live.contact_id = $1)
		     OR ($2::uuid IS NOT NULL AND live.lead_id = $2))
		   AND live.revoked_at IS NULL
		   AND NOT EXISTS (
		         SELECT 1 FROM communication_override held
		          WHERE (($3::uuid IS NOT NULL AND held.contact_id = $3)
		              OR ($4::uuid IS NOT NULL AND held.lead_id = $4))
		            AND held.category = live.category
		            AND held.revoked_at IS NULL
		            AND coalesce(array_position($6::text[], held.decided_by_level), array_length($6::text[], 1) + 1)
		                >= coalesce(array_position($6::text[], live.decided_by_level), array_length($6::text[], 1) + 1))
		 ORDER BY live.category,
		          coalesce(array_position($6::text[], live.decided_by_level), array_length($6::text[], 1) + 1) DESC,
		          live.recorded_at DESC
		RETURNING category, decided_by_level`,
		zeroAsNull(from.ContactID.UUID), zeroAsNull(from.LeadID.UUID),
		zeroAsNull(to.ContactID.UUID), zeroAsNull(to.LeadID.UUID), by, authorityLadder())
	if err != nil {
		return nil, fmt.Errorf("consent: carrying the subject's overrides onto the surviving record: %w", err)
	}
	defer rows.Close()
	var moved []carriedOverride
	for rows.Next() {
		var c carriedOverride
		if err := rows.Scan(&c.category, &c.level); err != nil {
			return nil, fmt.Errorf("consent: reading the carried overrides: %w", err)
		}
		moved = append(moved, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("consent: carrying the subject's overrides onto the surviving record: %w", err)
	}
	return moved, nil
}

// auditAndEmitCarriedOverrides records the merge's effect on the survivor and
// ships one event per carried row, the same shape CarryStopsTx uses and for
// the same reasons: AuditEvent because the survivor gained a row with no
// before-image, and CONTACT SURVIVOR ONLY because public-events.yaml declares
// consent.override_recorded with a static x-entity-type: contact — a lead
// survivor would ship an envelope naming contact:<lead uuid>, an id of the
// wrong kind.
func auditAndEmitCarriedOverrides(ctx context.Context, tx pgx.Tx, to commsauthz.StopSubject, moved []carriedOverride) error {
	entity, entityID := entityContact, to.ContactID.UUID
	if entityID.IsZero() {
		entity, entityID = entityLead, to.LeadID.UUID
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", entity, entityID,
		map[string]any{"overrides_carried": len(moved)})
	if err != nil {
		return err
	}
	if to.ContactID.IsZero() {
		return nil
	}
	for _, c := range moved {
		if err := storekit.EmitEvent(ctx, tx, auditID, entityID,
			overrideRecordedPayload(c.category, commsauthz.AuthorityLevel(c.level))); err != nil {
			return err
		}
	}
	return nil
}
