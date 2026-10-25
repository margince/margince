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

// unsplitAddressDataTask splits the company addresses older writers stored
// whole in line 1, with the splitter today's writers use. Text the splitter
// refuses stays as it is, so the task runs without an operator's review.
func unsplitAddressDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792852411_unsplit_addresses", Version: 1,
		Title: "Splitting company addresses stored in one line into their parts",
		Steps: []datatask.Step{{
			Name: "companies", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateUnsplitAddresses, Batch: repairUnsplitAddressesBatch,
		}},
	}
}

// repairUnsplitAddressesBatch walks every company by id, a page at a time. A
// batch inspects at most its limit, however few companies need a split.
func repairUnsplitAddressesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.RepairUnsplitCompanyAddressesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
}

func estimateUnsplitAddresses(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountCompanyAddressRepairRows(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
