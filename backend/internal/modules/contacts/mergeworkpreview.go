// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) previewMergeClaims(ctx context.Context, tx pgx.Tx, source ids.UUID) ([]crmcontracts.ConversationClaim, error) {
	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM conversation_claim WHERE contact_id=@source LIMIT 501) affected`, pgx.StrictNamedArgs{mergeSourceKey: source}).Scan(&total); err != nil {
		return nil, err
	}
	if total == 0 {
		return []crmcontracts.ConversationClaim{}, nil
	}
	if total > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	claims, err := s.ClaimsForContact(ctx, tx, ids.From[ids.ContactKind](source), nil, 501)
	if err != nil {
		return nil, err
	}
	// The native merge carries historical claims too; a filtered list is not its complete scope.
	if len(claims) != total {
		return nil, apperrors.ErrNotFound
	}
	return claims, nil
}

func previewMergeHandoffs(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]SDRHandoff, error) {
	if err := auth.Require(ctx, HandoffObject, principal.ActionRead); err != nil {
		return nil, err
	}

	column := columnContactID
	if kind == companyEntity {
		column = columnCompanyID
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT id, lead_id, contact_id, company_id, submitted_by, assigned_to, status, reason_id, coalesce(note,''), deal_id, submitted_at::text, decided_at::text, version
 FROM sdr_handoff WHERE %s=@source ORDER BY id LIMIT 501`, column), pgx.StrictNamedArgs{mergeSourceKey: source})
	if err != nil {
		return nil, err
	}
	handoffs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SDRHandoff])
	if err != nil {
		return nil, err
	}
	if len(handoffs) == 0 {
		return handoffs, nil
	}
	if len(handoffs) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, handoff := range handoffs {
		if err := ensureHandoffTargetsVisible(ctx, tx, NewSDRHandoff{LeadID: handoff.LeadID, ContactID: handoff.ContactID, CompanyID: handoff.CompanyID}); err != nil {
			return nil, err
		}
		if handoff.DealID != nil {
			if err := auth.EnsureLinkTarget(ctx, tx, "deal", *handoff.DealID); err != nil {
				return nil, err
			}
		}
	}
	return handoffs, nil
}
