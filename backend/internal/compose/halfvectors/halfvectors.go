// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package halfvectors is the data task that moves the vectors stored at full
// precision to half precision. Postgres converts each stored value; no model
// is called.
package halfvectors

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// halfColumnMigration adds the column the task fills.
const halfColumnMigration = "core/1795241482"

// DataTask walks the embedding table in key order and converts each vector
// still stored at full precision.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795241490_vectors_at_half_precision", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795241490_vectors_at_half_precision"),
		Requires: []string{halfColumnMigration},
		Steps: []datatask.Step{{
			Name: "convert_vectors", Limit: 500, MinLimit: 20,
			Audit: datatask.AuditPolicy{
				Reason: "search writes the same half-precision vector when it embeds a record, without a separate audit",
			},
			Estimate: estimate,
			Batch:    batch,
		}},
	}
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	next, moved, done, err := search.ConvertToHalfPrecisionTx(ctx, tx, in.Cursor, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.BatchOut{Cursor: next, Scanned: moved, Changed: moved, Done: done}, nil
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := search.CountFullPrecisionVectors(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
