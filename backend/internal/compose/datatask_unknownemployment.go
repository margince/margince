// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func unknownEmploymentDataTask(pool *pgxpool.Pool) datatask.Task {
	store := newContactsStore(InstallationDB(pool)).WithConsumerMail(capture.MatcherTx)
	return datatask.Task{
		ID: "core/1793548000_unknown_employment_links", Version: 1,
		Title: "Correcting company links members and assistants added", Requires: []string{"core/1789354558"},
		Steps: []datatask.Step{{
			Name: "current_unknown_employment_links", Limit: 200, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateUnknownEmployment,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				if err := auth.RequireSystem(ctx); err != nil {
					return datatask.BatchOut{}, err
				}
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				page, err := store.RepairUnknownEmploymentTx(ctx, tx, after, in.Limit, in.RunID.String())
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
			},
		}},
	}
}

func estimateUnknownEmployment(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountUnknownEmploymentLinks(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
