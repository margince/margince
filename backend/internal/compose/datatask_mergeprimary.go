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

// mergeDroppedPrimaryDataTask gives a contact back the current employer a
// merge archived with a duplicate edge, by promoting the kept duplicate. A
// contact whose flag a user changed after the merge stays as it is.
func mergeDroppedPrimaryDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1793719006_merge_dropped_primary", Version: 1,
		Title: "Restoring current employers lost in a merge", Requires: []string{"core/1793682320"},
		Steps: []datatask.Step{{
			Name: "restore_merge_dropped_primaries", Limit: 200, MinLimit: 1, EmptyWhenDone: true,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMergeDroppedPrimaries, Batch: restoreMergeDroppedPrimariesBatch,
		}},
	}
}

func restoreMergeDroppedPrimariesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.ReconcileMergeDroppedPrimariesTx(ctx, tx, after, in.Limit, in.RunID.String())
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
}

func estimateMergeDroppedPrimaries(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountMergeDroppedPrimaries(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
