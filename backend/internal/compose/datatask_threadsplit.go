// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const threadBatchLimitArg = "limit"

func threadRepairApplier(pools ...*pgxpool.Pool) capture.ThreadRepairApplier {
	apply := capture.ThreadRepairApplier{
		Invalidate: invalidateThreadProofTx, AuthorizeEdge: activities.ThreadEdgeAuthorizedTx, Activity: splitThreadActivityTx, Delivery: splitThreadDeliveryTx, Verdict: inheritThreadSeatTx, Basis: inheritThreadBasisTx, Representative: repairThreadSeatTx, Hold: holdThreadSeatTx,
		Notify: func(ctx context.Context, tx pgx.Tx, id ids.ActivityID, key string) error {
			return activities.ThreadPublishedTx(threadRepairContext(ctx), tx, id, key)
		},
	}
	if len(pools) > 0 && pools[0] != nil {
		apply.PublishSeat = threadAttributionReconciler(pools[0])
	}
	return apply
}

func threadSplitDataTask(pools ...*pgxpool.Pool) datatask.Task {
	return datatask.Task{ID: "core/1793602001_subject_thread_boundaries", Version: 1, Title: "Separate correspondence at subject boundaries", Requires: []string{"core/1793602000"}, Steps: []datatask.Step{
		{Name: "restore_parent_order", Limit: 100, MinLimit: 1, Audit: datatask.AuditPolicy{Reason: "capture records the same sender-header reference evidence without a separate audit"}, Estimate: estimateThreadParentOriginals, Batch: restoreThreadParentsBatch},
		{Name: "queue_conversations", Limit: 100, MinLimit: 1, Audit: datatask.AuditPolicy{Reason: "queue rows are derived repair scheduling, without a domain mutation"}, Estimate: estimateThreadRepairs, Batch: queueThreadRepairsBatch},
		{Name: "repair_conversations", Limit: 100, MinLimit: 1, Retry: capture.RetryThreadRepairTx, Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: estimateQueuedThreadRepairs, Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			return repairThreadsWithApplier(ctx, tx, in, threadRepairApplier(pools...))
		}},
	}}
}

func queueThreadRepairsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id,thread_key FROM activity WHERE kind='email' AND thread_key IS NOT NULL AND id>@after ORDER BY id LIMIT @limit`, pgx.NamedArgs{dataTaskAfterArg: after, threadBatchLimitArg: in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	type candidate struct {
		ID  ids.UUID
		Key string
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	for _, c := range candidates {
		if err := capture.QueueThreadRepairTx(ctx, tx, c.Key); err != nil {
			return datatask.BatchOut{}, err
		}
		scanned = append(scanned, c.ID)
	}
	return datatask.AdvanceUUID(in, scanned, 0)
}

func repairThreadsWithApplier(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, apply capture.ThreadRepairApplier) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	var batches uint64
	if len(in.Cursor) > 0 {
		if err := json.Unmarshal(in.Cursor, &batches); err != nil || batches == ^uint64(0) {
			return datatask.BatchOut{}, &storekit.MalformedCursorError{}
		}
	}
	scanned, changed, done, err := capture.ThreadRepairBatchTx(ctx, tx, in.Limit, in.RunID, apply)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	if done {
		var unresolved, quarantined bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_thread_repair WHERE state<>'ready') OR EXISTS(SELECT 1 FROM capture_thread_invalidation),EXISTS(SELECT 1 FROM capture_thread_repair WHERE state='quarantined') OR EXISTS(SELECT 1 FROM capture_thread_invalidation WHERE state='quarantined')`).Scan(&unresolved, &quarantined); err != nil {
			return datatask.BatchOut{}, err
		}
		if quarantined && scanned == 0 {
			return datatask.BatchOut{}, errors.New("conversation repair needs attention; resolve the reported fault and retry this correction")
		}
		done = !unresolved
	}
	cursor, err := json.Marshal(batches + 1)
	return datatask.BatchOut{Cursor: cursor, Scanned: scanned, Changed: changed, Done: done}, err
}

func estimateThreadRepairs(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM(SELECT id FROM activity WHERE kind='email' AND thread_key IS NOT NULL LIMIT @cap) candidates`, pgx.NamedArgs{dataTaskCapArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

func estimateQueuedThreadRepairs(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM(SELECT source_key FROM capture_thread_pending UNION ALL SELECT id::text FROM capture_thread_repair UNION ALL SELECT activity_id::text FROM capture_thread_invalidation LIMIT @cap) candidates`, pgx.NamedArgs{dataTaskCapArg: maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
