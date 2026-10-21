// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const carriedWithdrawal = `a.state = 'withdrawn' AND b.state <> 'withdrawn'`

// ConsentCarryChange keeps consent lineage changes visible before records move.
type ConsentCarryChange struct {
	PurposeID    ids.UUID
	PurposeLabel string
	SourceState  string
	TargetBefore *string
	TargetAfter  string
}

// ConsentCarryPreview collects consent effects for the same operation the writer will execute.
type ConsentCarryPreview struct {
	States        []ConsentCarryChange
	ProofsRehomed int64
}

// Both subjects have been authorized by the caller. Archived purposes remain
// included because the writer carries their stored states too.
func previewConsentCarry(ctx context.Context, tx pgx.Tx, kind consentCarryKind, source, target ids.UUID) (ConsentCarryPreview, error) {
	spec, ok := consentCarries[kind]
	if !ok {
		return ConsentCarryPreview{}, apperrors.ErrInvalidArgument
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT a.purpose_id, p.label, a.state, b.state,
 CASE WHEN b.id IS NULL THEN a.state WHEN `+carriedWithdrawal+` THEN 'withdrawn' ELSE b.state END
 FROM contact_consent a JOIN consent_purpose p ON p.id = a.purpose_id
 LEFT JOIN contact_consent b ON b.%[2]s = @target AND b.purpose_id = a.purpose_id
 WHERE a.%[1]s = @source ORDER BY a.purpose_id LIMIT 501`, spec.from, spec.to),
		pgx.StrictNamedArgs{mergeSourceKey: source, mergeTargetKey: target})
	if err != nil {
		return ConsentCarryPreview{}, fmt.Errorf("read carried consent changes: %w", err)
	}
	states, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ConsentCarryChange])
	if err != nil {
		return ConsentCarryPreview{}, err
	}
	if len(states) > 500 {
		return ConsentCarryPreview{}, apperrors.ErrInvalidArgument
	}
	out := ConsentCarryPreview{States: states}
	if spec.rehomeProof {
		err := tx.QueryRow(ctx, storekit.SQLf("SELECT count(*) FROM consent_event WHERE %s = @source", spec.from), pgx.StrictNamedArgs{mergeSourceKey: source}).Scan(&out.ProofsRehomed)
		if err != nil {
			return ConsentCarryPreview{}, err
		}
	}
	return out, nil
}

func previewPromotionConsent(ctx context.Context, tx pgx.Tx, lead ids.LeadID, contact ids.ContactID) ([]ConsentCarryChange, error) {
	preview, err := previewConsentCarry(ctx, tx, consentCarryLeadPromotion, lead.UUID, contact.UUID)
	return preview.States, err
}
