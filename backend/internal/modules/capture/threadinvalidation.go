// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const threadInvalidationColumns = `activity_id,source_key,repair_id,after_repair,after_node,after_topic,proofs_cleared,run_id`

type threadInvalidation struct {
	Activity      ids.ActivityID
	Source        string
	Repair        *ids.UUID
	AfterRepair   ids.UUID
	AfterNode     ids.ActivityID
	AfterTopic    ids.ActivityID
	ProofsCleared bool
	Run           ids.UUID
}

func selectThreadInvalidationTx(ctx context.Context, tx pgx.Tx, component string) (threadInvalidation, bool, error) {
	var in threadInvalidation
	var row pgx.Row
	if component == "" {
		row = tx.QueryRow(ctx, `SELECT `+threadInvalidationColumns+` FROM capture_thread_invalidation WHERE activity_id=(SELECT activity_id FROM (
 (SELECT activity_id,queue_order FROM capture_thread_invalidation WHERE state='ready' ORDER BY queue_order LIMIT 1)
 UNION ALL (SELECT activity_id,queue_order FROM capture_thread_invalidation WHERE state='backoff' AND retry_after<=statement_timestamp() ORDER BY retry_after,queue_order LIMIT 1)
 ) eligible ORDER BY queue_order LIMIT 1)`)
	} else {
		row = tx.QueryRow(ctx, `SELECT `+threadInvalidationColumns+` FROM capture_thread_invalidation WHERE activity_id=(
 SELECT activity_id FROM (
 (SELECT activity_id,queue_order FROM capture_thread_invalidation WHERE source_key=@component AND (state='ready' OR (state='backoff' AND retry_after<=statement_timestamp())) ORDER BY queue_order LIMIT 1)
 UNION ALL (SELECT i.activity_id,i.queue_order FROM capture_thread_barrier own JOIN capture_thread_barrier source ON source.repair_id=own.repair_id
 JOIN capture_thread_invalidation i ON i.source_key=source.blocked_key WHERE own.blocked_key=@component AND (i.state='ready' OR (i.state='backoff' AND i.retry_after<=statement_timestamp())) ORDER BY i.queue_order LIMIT 1)
 UNION ALL (SELECT i.activity_id,i.queue_order FROM capture_thread_barrier own JOIN capture_thread_invalidation i ON i.repair_id=own.repair_id WHERE own.blocked_key=@component AND (i.state='ready' OR (i.state='backoff' AND i.retry_after<=statement_timestamp())) ORDER BY i.queue_order LIMIT 1)
 UNION ALL (SELECT i.activity_id,i.queue_order FROM capture_thread_barrier own JOIN capture_thread_repair_node n ON n.repair_id=own.repair_id
 JOIN capture_thread_invalidation i ON i.activity_id=n.activity_id WHERE own.blocked_key=@component AND (i.state='ready' OR (i.state='backoff' AND i.retry_after<=statement_timestamp())) ORDER BY i.queue_order LIMIT 1)
 ) candidates ORDER BY queue_order LIMIT 1)`, pgx.NamedArgs{threadArgComponent: component})
	}
	err := row.Scan(&in.Activity, &in.Source, &in.Repair, &in.AfterRepair, &in.AfterNode, &in.AfterTopic, &in.ProofsCleared, &in.Run)
	if errors.Is(err, pgx.ErrNoRows) {
		return in, false, nil
	}
	return in, err == nil, err
}

func invalidateThreadContentTx(ctx context.Context, tx pgx.Tx, in threadInvalidation, apply ThreadRepairApplier) error {
	// Erasure takes the activity lock before the inbox; the worker preserves that order.
	var locked ids.ActivityID
	if err := tx.QueryRow(ctx, `SELECT id FROM activity WHERE id=@id FOR UPDATE`, pgx.NamedArgs{"id": in.Activity}).Scan(&locked); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	ctx = principal.WithActor(principal.WithCorrelationID(ctx, in.Run), principal.Principal{Type: principal.PrincipalSystem, ID: ThreadRepairActor})
	if !in.ProofsCleared {
		return invalidateThreadProofUnitTx(ctx, tx, in, apply)
	}
	if in.Repair != nil {
		return clearInvalidatedThreadNodeTx(ctx, tx, in)
	}
	return nextInvalidatedThreadRepairTx(ctx, tx, in)
}

func invalidateThreadProofUnitTx(ctx context.Context, tx pgx.Tx, in threadInvalidation, apply ThreadRepairApplier) error {
	if apply.Invalidate == nil {
		return errors.New("capture: thread erasure proof invalidator is missing")
	}
	changed, err := apply.Invalidate(ctx, tx, in.Activity)
	if err != nil || changed {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_invalidation SET proofs_cleared=true WHERE activity_id=@id`, pgx.NamedArgs{"id": in.Activity})
	return err
}

func nextInvalidatedThreadRepairTx(ctx context.Context, tx pgx.Tx, in threadInvalidation) error {
	var job ids.UUID
	err := tx.QueryRow(ctx, `SELECT repair_id FROM (
 SELECT repair_id FROM capture_thread_repair_node WHERE activity_id=@id AND repair_id>@after
 UNION SELECT repair_id FROM capture_thread_barrier WHERE blocked_key=@source AND repair_id>@after
 ) affected ORDER BY repair_id LIMIT 1`, pgx.NamedArgs{"id": in.Activity, threadArgAfter: in.AfterRepair, threadArgSource: in.Source}).Scan(&job)
	if errors.Is(err, pgx.ErrNoRows) {
		return finishThreadInvalidationTx(ctx, tx, in)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET phase='refresh',after_activity=@zero,graph_stage='reset',graph_cursor=@zero,graph_cycle=@zero,graph_best_key='',graph_best_topic='',failures=0,
 state='backoff',retry_after=statement_timestamp() WHERE id=@job`, pgx.NamedArgs{threadArgJob: job, threadArgZero: ids.Nil}); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_invalidation SET repair_id=@job,after_node=@zero WHERE activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": in.Activity, threadArgZero: ids.Nil})
	return err
}

func clearInvalidatedThreadNodeTx(ctx context.Context, tx pgx.Tx, in threadInvalidation) error {
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT activity_id FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: *in.Repair, threadArgAfter: in.AfterNode}).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `UPDATE capture_thread_invalidation SET after_repair=repair_id,repair_id=NULL,after_node=@zero WHERE activity_id=@id`, pgx.NamedArgs{"id": in.Activity, threadArgZero: ids.Nil})
		return err
	}
	if err != nil {
		return err
	}
	if id == in.Activity {
		_, err = tx.Exec(ctx, `DELETE FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: *in.Repair, "id": id})
	} else {
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET own_topic='',ancestor_topic='',ancestor_distance=-1,frontier_key='',frontier_topic='',root_key=message_key,root_proven=false,graph_state='waiting',graph_pending=0,graph_after=@zero WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: *in.Repair, "id": id, threadArgZero: ids.Nil})
	}
	if err != nil {
		return err
	}
	if err := invalidateThreadTopicCacheTx(ctx, tx, id); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_invalidation SET after_node=@after WHERE activity_id=@id`, pgx.NamedArgs{"id": in.Activity, threadArgAfter: id})
	return err
}

func finishThreadInvalidationTx(ctx context.Context, tx pgx.Tx, in threadInvalidation) error {
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT activity_id FROM capture_thread_topic WHERE root_key=@source AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgSource: in.Source, threadArgAfter: in.AfterTopic}).Scan(&id)
	if err == nil {
		if err := invalidateThreadTopicCacheTx(ctx, tx, id); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE capture_thread_invalidation SET after_topic=@after WHERE activity_id=@id`, pgx.NamedArgs{"id": in.Activity, threadArgAfter: id})
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM capture_thread_pending WHERE source_key=@source`, pgx.NamedArgs{threadArgSource: in.Source}); err != nil {
		return err
	}
	if err := QueueThreadRepairTx(ctx, tx, in.Source, true); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM capture_thread_invalidation WHERE activity_id=@id`, pgx.NamedArgs{"id": in.Activity})
	return err
}
