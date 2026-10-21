// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

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
