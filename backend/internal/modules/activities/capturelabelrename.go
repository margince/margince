// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RenameCaptureLabelsTx changes only mutable derived labels, preserving edit versions.
func RenameCaptureLabelsTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := activitiesToInspectTx(ctx, tx, in)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	var changed int
	if err := tx.QueryRow(ctx, `SELECT public.rename_legacy_capture_labels(@ids::uuid[])`, pgx.NamedArgs{"ids": page}).Scan(&changed); err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: renaming capture labels: %w", err)
	}
	return datatask.AdvanceUUID(in, page, changed)
}

// CountActivitiesToInspect bounds a correction that walks the activity primary key.
func CountActivitiesToInspect(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT id FROM activity ORDER BY id LIMIT @limit) a`, pgx.NamedArgs{argLimit: maximum + 1}).Scan(&count)
	if err != nil {
		return 0, false, fmt.Errorf("activities: estimating capture label scan: %w", err)
	}
	return min(count, maximum), count > maximum, nil
}

func activitiesToInspectTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) ([]ids.UUID, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM activity WHERE id > @after ORDER BY id LIMIT @limit`, pgx.NamedArgs{"after": after, argLimit: in.Limit})
	if err != nil {
		return nil, fmt.Errorf("activities: paging stored activities: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
