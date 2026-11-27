// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"github.com/margince/margince/backend/internal/shared/apperrors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergeCompanySingleton makes competing company settings visible before a merge.
type MergeCompanySingleton struct {
	Kind     string
	Retained bool
}

func previewCompanySingletons(ctx context.Context, tx pgx.Tx, source, target ids.UUID) ([]MergeCompanySingleton, error) {
	if err := refuseLiveIntoEndedPartner(ctx, tx, source, target); err != nil {
		return nil, err
	}
	var sourcePartner, targetPartner, sourceVAT, targetVAT bool
	if err := tx.QueryRow(ctx, `SELECT
 EXISTS (SELECT 1 FROM partner WHERE company_id=@source),
 EXISTS (SELECT 1 FROM partner WHERE company_id=@target),
 EXISTS (SELECT 1 FROM company_vat_check WHERE company_id=@source),
 EXISTS (SELECT 1 FROM company_vat_check WHERE company_id=@target)`,
		pgx.StrictNamedArgs{mergeSourceKey: source, mergeTargetKey: target}).Scan(&sourcePartner, &targetPartner, &sourceVAT, &targetVAT); err != nil {
		return nil, err
	}
	var out []MergeCompanySingleton
	if sourcePartner {
		if err := auth.Require(ctx, "partner", principal.ActionRead); err != nil {
			return nil, err
		}
		out = append(out, MergeCompanySingleton{Kind: "partner", Retained: targetPartner})
	}
	if sourceVAT {
		if err := previewFieldsVisible(ctx, companyEntity, "vat_number"); err != nil {
			return nil, err
		}
		out = append(out, MergeCompanySingleton{Kind: "vat_check", Retained: targetVAT})
	}
	return out, nil
}

// refuseLiveIntoEndedPartner prevents an ended survivor from replacing live
// commission terms while the merge moves the source's deals to that survivor.
func refuseLiveIntoEndedPartner(ctx context.Context, tx pgx.Tx, source, target ids.UUID) error {
	var losesLive bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM partner src JOIN partner dst ON dst.company_id = $2 WHERE src.company_id = $1 AND src.archived_at IS NULL AND dst.archived_at IS NOT NULL)`, source, target).Scan(&losesLive); err != nil {
		return err
	}
	if !losesLive {
		return nil
	}
	if err := auth.Require(ctx, "partner", principal.ActionRead); err != nil {
		return err
	}
	return fmt.Errorf("keep the company with the live partner program as the merge survivor so its commission terms remain in force: %w", apperrors.ErrConflict)
}
