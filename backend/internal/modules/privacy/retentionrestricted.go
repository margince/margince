// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The expiry of a restriction (A165/ADR-0114 §2): when a held record's
// statutory window closes, the erasure it suspended completes without anybody
// asking again. This is the ONE path allowed to write a restricted row, and it
// takes the only shape the data-layer guard admits — the lift and the erasure
// in a single statement.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// restrictionExpiryStages is the batched stage this file adds to a workspace
// pass, claiming its own retentionBatch like the AI sweeps. It has its own
// count rather than riding aiRetentionStages: that name says what those
// stages are, and the erasure of expired Handelsbriefe is not one of them.
const restrictionExpiryStages = 1

// restrictionExpiredCause is what the tombstone and the event say completed
// the erasure — the window ran out, nobody decided anything.
const restrictionExpiredCause = "restriction_expired"

// evaluateRestrictionExpiry completes the suspended erasure of every held
// record whose window has closed. It runs IRRESPECTIVE of the retain-only
// posture: that posture suspends the storage-limitation ladder an operator
// authored, and this is not a policy the operator may decline — it is the
// second half of an Art. 17 request the engine already accepted and held.
//
// A record under a legal hold reached through ANY of its links is skipped:
// the hold outranks the subject's request until it is lifted, and the row
// stays restricted meanwhile, which is the more protective of the two states.
// The person arm is included here where the erasure's own selectors leave it
// out — the erasure proved its subject unheld before it ran, but a hold can
// land on that (now anonymised) person row during the years the window is
// open, and the sweep must see it. The predicate is repeated in the lift
// statement itself so a hold placed between the selection and the write
// still wins.
func (s *RetentionService) evaluateRestrictionExpiry(ctx context.Context) error {
	var due []ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id FROM activity a
			WHERE a.restricted_at IS NOT NULL AND a.restricted_until <= now()
			`+notHeldThroughAnyLink("a.id")+`
			ORDER BY a.restricted_until
			LIMIT $1`, retentionBatch)
		if err != nil {
			return err
		}
		due, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return fmt.Errorf("retention restriction expiry: select: %w", err)
	}
	for _, id := range due {
		if err := s.expireRestriction(ctx, id); err != nil {
			return fmt.Errorf("retention restriction expiry on %s: %w", id, err)
		}
	}
	return nil
}

// notHeldThroughAnyLink is notTransitivelyHeld plus the person arm.
func notHeldThroughAnyLink(activityID string) string {
	return `
	  AND NOT EXISTS (
	    SELECT 1 FROM activity_link h
	    LEFT JOIN person hp ON hp.id = h.person_id
	    LEFT JOIN organization org ON org.id = h.organization_id
	    LEFT JOIN deal dl ON dl.id = h.deal_id
	    LEFT JOIN lead ld ON ld.id = h.lead_id
	    LEFT JOIN project pj ON pj.id = h.project_id
	    WHERE h.activity_id = ` + activityID + `
	      AND (coalesce(hp.legal_hold, false) OR coalesce(org.legal_hold, false) OR coalesce(dl.legal_hold, false)
	           OR coalesce(ld.legal_hold, false) OR coalesce(pj.legal_hold, false)))`
}

// expireRestriction erases one held record in its own audited transaction. The
// `restricted_until <= now()` predicate is its CAS: a rival sweep that already
// completed this row matches nothing, and nothing is audited twice for one
// erasure.
func (s *RetentionService) expireRestriction(ctx context.Context, id ids.UUID) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		class, err := liftAndEraseHeldRecord(ctx, tx, id,
			`AND a.restricted_until <= now() `+notHeldThroughAnyLink("a.id"))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := s.eraser.purgeContentDerivedFrom(ctx, tx, id); err != nil {
			return err
		}
		auditID, err := storekit.AuditWithEvidence(ctx, tx, actionExpire, "activity", id, nil, nil, map[string]any{
			evidenceKeyCause: restrictionExpiredCause, evidenceKeyClass: class, evidenceKeyBasis: statutoryBasisCorrespondence,
		})
		if err != nil {
			return err
		}
		reason := restrictionExpiredCause
		return storekit.EmitEventForEntity(ctx, tx, auditID, "activity", id, retentionAppliedPayload(actionErase, nil, &reason))
	})
}

// liftAndEraseHeldRecord is the ONE statement that ends a restriction, shared
// by the expiry sweep and the controller's release, which differ only in what
// makes the record due. Two copies is how the two paths would come to destroy
// different things — a defect this file has already shipped once, in the
// counterparty_email that migration 0291 had to add to a guard written from
// the smaller of two content lists.
//
// The lift and the erasure are one statement because the data-layer guard
// admits nothing else (0290): a lift that left the body readable would undo
// the restriction and keep the data. due is the caller's extra predicate over
// the activity aliased `a`; the shared arm is `restricted_at IS NOT NULL`, so
// a record nobody is holding matches nothing and answers pgx.ErrNoRows for the
// caller to interpret.
func liftAndEraseHeldRecord(ctx context.Context, tx pgx.Tx, id ids.UUID, due string) (class string, err error) {
	err = tx.QueryRow(ctx, `
		UPDATE activity a
		   SET restricted_at = NULL, restricted_reason = NULL, restricted_until = NULL,
		       subject = NULL, body = NULL, raw = NULL, counterparty_email = NULL,
		       redacted_fields = a.redacted_fields || ARRAY(SELECT c FROM unnest(ARRAY[
		           CASE WHEN a.subject IS NOT NULL THEN 'subject' END,
		           CASE WHEN a.body IS NOT NULL THEN 'body' END]) AS c
		         WHERE c IS NOT NULL),
		       archived_at = coalesce(a.archived_at, now())
		 WHERE a.id = $1 AND a.restricted_at IS NOT NULL `+due+`
		 RETURNING a.retention_class`, id).Scan(&class)
	return class, err
}
