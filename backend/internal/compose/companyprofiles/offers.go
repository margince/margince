// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyprofiles

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// OfferDuplicatesTask registers the bounded correction for site-read offer mirrors.
func OfferDuplicatesTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794714000_company_offer_duplicates", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794714000_company_offer_duplicates"),
		Steps: []datatask.Step{{
			Name: "company_offer", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateCompanyOffers, Batch: clearCompanyOffersBatch,
		}},
	}
}

func clearCompanyOffersBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.ClearSiteOfferDuplicatesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
}

func estimateCompanyOffers(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountSiteOfferDuplicates(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
