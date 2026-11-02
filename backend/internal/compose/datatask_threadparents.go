// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var threadParentOriginalScope = `FROM activity a JOIN raw_capture rc ON ` + storedMailOriginal + ` WHERE a.kind='email' AND ` + storedMailProvider

func restoreThreadParentsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id,rc.payload `+threadParentOriginalScope+` AND a.id>@after ORDER BY a.id LIMIT @limit`, pgx.NamedArgs{dataTaskAfterArg: after, threadBatchLimitArg: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	originals, err := pgx.CollectRows(rows, pgx.RowToStructByPos[mailRecipientOriginal])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(originals))
	for _, original := range originals {
		raw, err := capture.DecodeStoredOriginal(original.Payload)
		if err != nil {
			return datatask.BatchOut{}, fmt.Errorf("stored parent evidence is unreadable: %w", apperrors.ErrInvalidArgument)
		}
		parsed, err := mailmap.Parse(raw, "")
		if err != nil {
			return datatask.BatchOut{}, fmt.Errorf("stored parent evidence is unreadable: %w", apperrors.ErrInvalidArgument)
		}
		if err := activities.RecordMailReferencesTx(ctx, tx, ids.From[ids.ActivityKind](original.ID), parsed.ToRecord("gmail", nil).ReplyTo); err != nil {
			return datatask.BatchOut{}, err
		}
		scanned = append(scanned, original.ID)
	}
	return datatask.AdvanceUUID(in, scanned, 0)
}

func estimateThreadParentOriginals(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM(SELECT a.id `+threadParentOriginalScope+` LIMIT @cap) originals`, pgx.NamedArgs{dataTaskCapArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
