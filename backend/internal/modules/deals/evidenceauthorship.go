// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// evidenceAuthorshipBatch bounds one reconciliation transaction.
const evidenceAuthorshipBatch = 200

// EvidenceAuthorshipScope narrows a reconciliation. Domain, when set, keeps
// the evidence an address at that domain or a subdomain wrote. Those are the
// claims an own-domain change can move.
type EvidenceAuthorshipScope struct {
	Domain string
}

// ReconcileEvidenceAuthorship judges again who wrote each activity behind
// live stage evidence, against the current own domains. The side was judged
// at the write, so an own domain added or removed since leaves it wrong.
// A changed side is corrected through the audited writer, and the stage
// policy, the approval check and assurance read it again.
//
// Refuted rows stay as a member left them. Pages are read unlocked, and only a
// row being corrected is locked. It answers how many rows it corrected.
func (s *Store) ReconcileEvidenceAuthorship(ctx context.Context, own OwnDomainReader, scope EvidenceAuthorshipScope) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	scope.Domain = owndomain.Fold(scope.Domain)
	corrected := 0
	var after ids.UUID
	for {
		var scanned int
		err := s.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			var changed int
			scanned, changed, after, err = reconcileEvidenceAuthorshipTx(ctx, tx, own, scope, after)
			corrected += changed
			return err
		})
		if err != nil {
			return corrected, err
		}
		if scanned < evidenceAuthorshipBatch {
			return corrected, nil
		}
	}
}

type activityEvidence struct {
	ID       ids.UUID
	DealID   ids.UUID
	SourceID ids.UUID
	Side     string
}

// evidenceAuthorshipPage selects live activity-backed evidence whose side the
// own domains can decide: the activity did not go out from here.
var evidenceAuthorshipPage = `
	SELECT ev.id, ev.deal_id, ev.source_id, ev.author_side
	  FROM deal_stage_evidence ev
	  JOIN activity a ON a.id = ev.source_id
	 WHERE ev.source_type = $1 AND ev.refuted_at IS NULL AND ev.id > $2
	   AND a.direction IS DISTINCT FROM 'outbound'
	   AND ($4 = '' OR EXISTS (SELECT 1 FROM activity_participant p
	         WHERE p.activity_id = a.id AND p.role IN ('from', 'organizer') AND p.user_id IS NULL
	           AND ` + owndomain.CoversSQL("p.address_domain_folded", "$4") + `))
	 ORDER BY ev.id LIMIT $3`

// reconcileEvidenceAuthorshipTx corrects one page of evidence after the
// cursor. It answers the rows it inspected, the rows it changed and the next
// cursor.
func reconcileEvidenceAuthorshipTx(
	ctx context.Context, tx pgx.Tx, own OwnDomainReader, scope EvidenceAuthorshipScope, after ids.UUID,
) (int, int, ids.UUID, error) {
	rows, err := tx.Query(ctx, evidenceAuthorshipPage, SourceActivity, after, evidenceAuthorshipBatch, scope.Domain)
	if err != nil {
		return 0, 0, after, fmt.Errorf("deals: reading evidence to judge its author again: %w", err)
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[activityEvidence])
	if err != nil {
		return 0, 0, after, fmt.Errorf("deals: reading evidence to judge its author again: %w", err)
	}
	if len(page) == 0 {
		return 0, 0, after, nil
	}
	domains, err := own.Domains(ctx, tx)
	if err != nil {
		return 0, 0, after, err
	}
	sides := map[ids.UUID]AuthorSide{}
	changed := 0
	for _, ev := range page {
		after = ev.ID
		side, known := sides[ev.SourceID]
		if !known {
			judged, found, err := judgeActivityAuthor(ctx, tx, ev.SourceID, domains)
			if err != nil {
				return len(page), changed, after, err
			}
			if !found {
				continue
			}
			side = judged
			sides[ev.SourceID] = side
		}
		if string(side) == ev.Side {
			continue
		}
		corrected, err := correctEvidenceAuthor(ctx, tx, ev, side)
		if err != nil {
			return len(page), changed, after, err
		}
		if corrected {
			changed++
		}
	}
	return len(page), changed, after, nil
}

// judgeActivityAuthor is AuthorSideOf over the activity as stored, archived
// or not and linked or not: the stage policy reads the evidence either way.
// found is false when the activity no longer exists.
func judgeActivityAuthor(ctx context.Context, tx pgx.Tx, activityID ids.UUID, domains []string) (AuthorSide, bool, error) {
	var direction *string
	err := tx.QueryRow(ctx, `SELECT direction FROM activity WHERE id = $1`, activityID).Scan(&direction)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("deals: reading an evidence source's direction: %w", err)
	}
	participants, err := readActivityParticipants(ctx, tx, activityID)
	if err != nil {
		return "", false, err
	}
	var dir string
	if direction != nil {
		dir = *direction
	}
	return AuthorSideOf(dir, participants, domains), true, nil
}

// correctEvidenceAuthor writes the side the current own domains give, with
// the audit row and the deal event every evidence change carries. It writes
// nothing when the row changed or was refuted since the page was read.
func correctEvidenceAuthor(ctx context.Context, tx pgx.Tx, ev activityEvidence, side AuthorSide) (bool, error) {
	lock, err := storekit.LockRow(ctx, tx, evidenceEntity, ev.ID, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var current string
	var refuted bool
	if err := tx.QueryRow(ctx, `SELECT author_side, refuted_at IS NOT NULL FROM deal_stage_evidence WHERE id = $1`, ev.ID).
		Scan(&current, &refuted); err != nil {
		return false, fmt.Errorf("deals: rereading an evidence row under its lock: %w", err)
	}
	if current != ev.Side || refuted {
		return false, nil
	}
	patch := storekit.NewPatch()
	patch.Set("author_side", ev.Side, string(side))
	if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
		return false, fmt.Errorf("deals: correcting an evidence author: %w", err)
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", evidenceEntity, ev.ID,
		patch.Before(), patch.After(), map[string]any{"rejudged_against": "own_domains"})
	if err != nil {
		return false, fmt.Errorf("deals: auditing a corrected evidence author: %w", err)
	}
	return true, emitEvidenceChanged(ctx, tx, auditID, ids.From[ids.DealKind](ev.DealID))
}
