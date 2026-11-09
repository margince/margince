// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mailCopiesDataTask(pool *pgxpool.Pool) datatask.Task {
	store := activities.NewStore(InstallationDB(pool))
	return datatask.Task{ID: "core/1794132000_fold_proven_mail_copies", Version: 1, Title: "Combining proven copies of imported mail", Steps: []datatask.Step{{
		Name: "fold_mail_copies", Limit: 50, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: activities.CountMailCopies,
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			copies, err := activities.MailCopiesTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			scanned := make([]ids.UUID, 0, len(copies))
			changed := 0
			for _, copy := range copies {
				scanned = append(scanned, copy.ID)
				folded, err := store.FoldMailCopyTx(ctx, tx, copy, capture.ProvedUnambiguouslyTx, carryMailImportsTx)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				if folded {
					changed++
				}
			}
			return datatask.AdvanceUUID(in, scanned, changed)
		},
	}}}
}

func carryMailImportsTx(ctx context.Context, tx pgx.Tx, duplicate, survivor ids.ActivityID) (bool, error) {
	var args []any
	arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	duplicateArg, survivorArg := arg(duplicate), arg(survivor)
	var conflict bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.archived_at IS NULL AND a.restricted_at IS NULL AND a.id=`+duplicateArg+` AND (a.audience='selected' OR a.audience_reason='manual' OR (a.audience<>'workspace' AND NOT EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id=a.id)))) OR EXISTS (SELECT 1 FROM capture_import other JOIN capture_import kept ON kept.activity_id=`+survivorArg+` AND kept.user_id=other.user_id WHERE other.activity_id=`+duplicateArg+` AND NOT `+sameImportAccess+`)`, args...).Scan(&conflict)
	if err != nil || conflict {
		return false, err
	}
	rows, err := tx.Query(ctx, `SELECT user_id FROM capture_import WHERE activity_id=$1 ORDER BY user_id`, duplicate)
	if err != nil {
		return false, err
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, err
	}
	for _, seat := range seats {
		if err := capture.CarryImportTx(ctx, tx, duplicate, survivor, seat); err != nil {
			return false, err
		}
	}
	return true, nil
}
