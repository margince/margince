// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Each ready parent visits its children by an indexed cursor, including wide stars.
func descendThreadGraphUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	parent, err := scanThreadNode(tx.QueryRow(ctx, `SELECT `+threadNodeColumns+` FROM capture_thread_repair_node WHERE repair_id=@job AND graph_state='ready' ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID}))
	if errors.Is(err, pgx.ErrNoRows) {
		phase := threadPhaseLabels
		if job.Phase == threadPhaseRoots {
			phase = "targets"
		}
		if err := resetThreadGraphStageTx(ctx, tx, job.ID, threadGraphResetStage); err != nil {
			return err
		}
		return threadRepairPhaseTx(ctx, tx, job.ID, phase)
	}
	if err != nil {
		return err
	}
	var after ids.ActivityID
	if err := tx.QueryRow(ctx, `SELECT graph_after FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": parent.ID}).Scan(&after); err != nil {
		return err
	}
	child, err := scanThreadNode(tx.QueryRow(ctx, `SELECT `+threadNodeColumns+` FROM capture_thread_repair_node WHERE repair_id=@job AND parent_activity_id=@parent AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID, threadArgParent: parent.ID, threadArgAfter: after}))
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state='done' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": parent.ID})
		return err
	}
	if err != nil {
		return err
	}
	if err := descendThreadGraphEdgeTx(ctx, tx, job, parent, child); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_after=@after WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": parent.ID, threadArgAfter: child.ID})
	return err
}

func descendThreadGraphEdgeTx(ctx context.Context, tx pgx.Tx, job threadRepair, parent, child threadRepairNode) error {
	roots := job.Phase == threadPhaseRoots
	if !roots && child.State.OwnTopic != "" {
		return nil
	}
	if roots && (child.State.RootProven || child.State.topic() != parent.State.topic()) {
		return nil
	}
	if roots {
		var state string
		if err := tx.QueryRow(ctx, `SELECT graph_state FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": child.ID}).Scan(&state); err != nil {
			return err
		}
		if state != "waiting" {
			return nil
		}
		child.State.RootKey, child.State.RootProven = parent.State.RootKey, parent.State.RootProven
	} else {
		child.State = advanceThreadAncestor(child.State, parent.State)
	}
	if err := writeThreadStateTx(ctx, tx, job.ID, child); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE capture_thread_repair_node SET graph_state='ready' WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job.ID, "id": child.ID})
	return err
}
