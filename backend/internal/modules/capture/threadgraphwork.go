// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	threadGraphResetStage   = "reset"
	threadGraphSeedStage    = "seed"
	threadGraphUpStage      = "up"
	threadGraphDownStage    = "down"
	threadGraphCollectStage = "collect"
	threadGraphWriteStage   = "write"

	threadGraphCountStage    = "count"
	threadGraphDownSeedStage = "down_seed"
)

type threadGraphWork struct {
	Stage     string
	Cursor    ids.ActivityID
	Cycle     ids.ActivityID
	BestKey   string
	BestTopic string
}

func advanceThreadGraphUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	var work threadGraphWork
	err := tx.QueryRow(ctx, `SELECT graph_stage,graph_cursor,graph_cycle,graph_best_key,graph_best_topic FROM capture_thread_repair WHERE id=@job`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&work.Stage, &work.Cursor, &work.Cycle, &work.BestKey, &work.BestTopic)
	if err != nil {
		return err
	}
	switch work.Stage {
	case threadGraphResetStage, threadGraphCountStage, threadGraphSeedStage, threadGraphDownSeedStage:
		return prepareThreadGraphUnitTx(ctx, tx, job, work.Stage)
	case threadGraphUpStage:
		return trimThreadGraphUnitTx(ctx, tx, job)
	case threadGraphCollectStage, threadGraphWriteStage:
		return cycleThreadGraphUnitTx(ctx, tx, job, work)
	case threadGraphDownStage:
		return descendThreadGraphUnitTx(ctx, tx, job)
	default:
		return fmt.Errorf("capture: unknown thread graph stage %q", work.Stage)
	}
}

func nextThreadGraphNodeTx(ctx context.Context, tx pgx.Tx, job threadRepair) (threadRepairNode, error) {
	return scanThreadNode(tx.QueryRow(ctx, `SELECT `+threadNodeColumns+` FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID, threadArgAfter: job.After}))
}

func advanceThreadGraphCursorTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID) error {
	_, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET after_activity=@after WHERE id=@job`, pgx.NamedArgs{threadArgJob: job, threadArgAfter: id})
	return err
}

func resetThreadGraphStageTx(ctx context.Context, tx pgx.Tx, job ids.UUID, stage string) error {
	_, err := tx.Exec(ctx, `UPDATE capture_thread_repair SET graph_stage=@stage,after_activity=@zero,graph_cursor=@zero,graph_cycle=@zero,graph_best_key='',graph_best_topic='' WHERE id=@job`, pgx.NamedArgs{threadArgJob: job, "stage": stage, threadArgZero: ids.Nil})
	return err
}

func prepareThreadGraphUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, stage string) error {
	node, err := nextThreadGraphNodeTx(ctx, tx, job)
	if errors.Is(err, pgx.ErrNoRows) {
		next := map[string]string{threadGraphResetStage: threadGraphCountStage, threadGraphCountStage: threadGraphSeedStage, threadGraphSeedStage: threadGraphUpStage, threadGraphDownSeedStage: threadGraphDownStage}[stage]
		return resetThreadGraphStageTx(ctx, tx, job.ID, next)
	}
	if err != nil {
		return err
	}
	args := pgx.NamedArgs{threadArgJob: job.ID, "id": node.ID, threadArgZero: ids.Nil}
	switch stage {
	case threadGraphResetStage:
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_pending=0,graph_state='waiting',graph_after=@zero WHERE repair_id=@job AND activity_id=@id`, args)
	case threadGraphCountStage:
		err = countThreadGraphEdgeTx(ctx, tx, job, node)
	case threadGraphSeedStage:
		fixed := node.State.OwnTopic != ""
		if job.Phase == threadPhaseRoots {
			fixed = node.State.RootProven
		}
		args["fixed"] = fixed
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state=CASE WHEN @fixed OR graph_pending=0 THEN 'ready' ELSE 'waiting' END WHERE repair_id=@job AND activity_id=@id`, args)
	case threadGraphDownSeedStage:
		fixed := node.State.OwnTopic != ""
		if job.Phase == threadPhaseRoots {
			args["fixed"] = node.State.RootProven
			_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state=CASE WHEN @fixed OR graph_state='cycle' THEN 'ready' ELSE 'waiting' END,graph_after=@zero WHERE repair_id=@job AND activity_id=@id`, args)
		} else {
			_, found, parentErr := optionalThreadParentTx(ctx, tx, job.ID, node.Parent)
			if parentErr != nil {
				return parentErr
			}
			args["fixed"] = fixed || !found
			_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state=CASE WHEN @fixed THEN 'ready' ELSE 'waiting' END,graph_after=@zero WHERE repair_id=@job AND activity_id=@id`, args)
		}
	}
	if err != nil {
		return err
	}
	return advanceThreadGraphCursorTx(ctx, tx, job.ID, node.ID)
}

func threadGraphEdge(child, parent threadRepairNode, roots bool) bool {
	if roots {
		return !parent.State.RootProven && child.State.topic() == parent.State.topic()
	}
	return parent.State.OwnTopic == ""
}

func countThreadGraphEdgeTx(ctx context.Context, tx pgx.Tx, job threadRepair, node threadRepairNode) error {
	parent, found, err := optionalThreadParentTx(ctx, tx, job.ID, node.Parent)
	if err != nil || !found {
		return err
	}
	if !threadGraphEdge(node, parent, job.Phase == threadPhaseRoots) {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_pending=graph_pending+1 WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": parent.ID})
	return err
}
