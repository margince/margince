// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// companyIndustryDataTask gives every company that holds industry text the code
// the text names. Text that names no code leaves the industry empty and is kept
// in the description. The mapping is a fixed table and nothing is deleted, so
// the task runs without an operator's review.
func companyIndustryDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792811260_company_industry_codes", Version: 1,
		Title: "Mapping company industries onto the fixed list", Requires: []string{"core/1792799323"},
		Steps: []datatask.Step{{
			Name: "company_industry", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateCompanyIndustries, Batch: remapCompanyIndustriesBatch,
		}},
	}
}

func remapCompanyIndustriesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.RemapCompanyIndustriesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
}

func estimateCompanyIndustries(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountCompaniesWithLegacyIndustry(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
