// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Trimming consumes each edge once; only homogeneous cycles remain without ready leaves.
func trimThreadGraphUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	child, err := scanThreadNode(tx.QueryRow(ctx, `SELECT `+threadNodeColumns+` FROM capture_thread_repair_node WHERE repair_id=@job AND graph_state='ready' ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID}))
	if errors.Is(err, pgx.ErrNoRows) {
		return startThreadGraphCycleTx(ctx, tx, job)
	}
	if err != nil {
		return err
	}
	parent, found, err := optionalThreadParentTx(ctx, tx, job.ID, child.Parent)
	if err != nil {
		return err
	}
	if found && threadGraphEdge(child, parent, job.Phase == threadPhaseRoots) {
		if job.Phase == threadPhaseRoots {
			_, parent.State = advanceThreadRoots(child.State, parent.State)
		} else {
			parent.State = advanceThreadFrontier(parent.State, child.State)
		}
		if err := writeThreadStateTx(ctx, tx, job.ID, parent); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_pending=graph_pending-1,graph_state=CASE WHEN graph_pending=1 THEN 'ready' ELSE graph_state END WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": parent.ID})
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state='done' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": child.ID})
	return err
}

func startThreadGraphCycleTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT activity_id FROM capture_thread_repair_node WHERE repair_id=@job AND graph_state='waiting' ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return resetThreadGraphStageTx(ctx, tx, job.ID, threadGraphDownSeedStage)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair SET graph_stage='collect',graph_cursor=@id,graph_cycle=@id,graph_best_key='',graph_best_topic='' WHERE id=@job`, pgx.NamedArgs{threadArgJob: job.ID, "id": id})
	return err
}

func cycleThreadGraphUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair, work threadGraphWork) error {
	node, err := readThreadNodeTx(ctx, tx, job.ID, work.Cursor)
	if err != nil {
		return err
	}
	if node.Parent == nil {
		return errors.New("capture: unfinished thread graph has no parent")
	}
	if work.Stage == threadGraphCollectStage {
		key, topic := node.State.FrontierKey, node.State.FrontierTopic
		if job.Phase == threadPhaseRoots {
			key = node.State.RootKey
		}
		if key != "" && (work.BestKey == "" || key < work.BestKey) {
			work.BestKey, work.BestTopic = key, topic
		}
		if *node.Parent == work.Cycle {
			work.Stage = threadGraphWriteStage
		}
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair SET graph_stage=@stage,graph_cursor=@next,graph_best_key=@key,graph_best_topic=@topic WHERE id=@job`, pgx.NamedArgs{threadArgJob: job.ID, "stage": work.Stage, "next": *node.Parent, threadArgKey: work.BestKey, threadArgTopic: work.BestTopic})
		return err
	}
	if job.Phase == threadPhaseRoots {
		node.State.RootKey = work.BestKey
	} else {
		node.State.FrontierKey, node.State.FrontierTopic = work.BestKey, work.BestTopic
	}
	if err := writeThreadStateTx(ctx, tx, job.ID, node); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state='cycle' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": node.ID}); err != nil {
		return err
	}
	if *node.Parent == work.Cycle {
		return resetThreadGraphStageTx(ctx, tx, job.ID, threadGraphUpStage)
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair SET graph_cursor=@next WHERE id=@job`, pgx.NamedArgs{threadArgJob: job.ID, "next": *node.Parent})
	return err
}
