// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const threadNodeColumns = `activity_id,source_key,parent_key,parent_activity_id,message_key,own_topic,ancestor_topic,ancestor_distance,frontier_key,frontier_topic,root_key,root_proven,observed_version`

func scanThreadNode(row pgx.Row) (threadRepairNode, error) {
	var n threadRepairNode
	err := row.Scan(&n.ID, &n.Source, &n.ParentKey, &n.Parent, &n.State.Key, &n.State.OwnTopic, &n.State.AncestorTopic, &n.State.AncestorDistance, &n.State.FrontierKey, &n.State.FrontierTopic, &n.State.RootKey, &n.State.RootProven, &n.Version)
	return n, err
}

func readThreadNodeTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id ids.ActivityID) (threadRepairNode, error) {
	return scanThreadNode(tx.QueryRow(ctx, `SELECT `+threadNodeColumns+` FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": id}))
}

func writeThreadStateTx(ctx context.Context, tx pgx.Tx, job ids.UUID, n threadRepairNode) error {
	_, err := tx.Exec(ctx, `UPDATE capture_thread_repair_node SET ancestor_topic=@ancestor,ancestor_distance=@distance,frontier_key=@frontier,frontier_topic=@topic,root_key=@root,root_proven=@proven WHERE repair_id=@job AND activity_id=@id`, pgx.NamedArgs{threadArgJob: job, "id": n.ID, "ancestor": n.State.AncestorTopic, "distance": n.State.AncestorDistance, "frontier": n.State.FrontierKey, threadArgTopic: n.State.FrontierTopic, threadArgRoot: n.State.RootKey, "proven": n.State.RootProven})
	return err
}

func advanceThreadRepairUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	if job.Phase != threadPhaseLabels {
		return advanceThreadGraphUnitTx(ctx, tx, job)
	}
	child, err := nextThreadGraphNodeTx(ctx, tx, job)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := resetThreadGraphStageTx(ctx, tx, job.ID, threadGraphResetStage); err != nil {
			return err
		}
		return threadRepairPhaseTx(ctx, tx, job.ID, threadPhaseRoots)
	}
	if err != nil {
		return err
	}
	parent, found, err := optionalThreadParentTx(ctx, tx, job.ID, child.Parent)
	if err != nil {
		return err
	}
	var state *threadBoundaryState
	if found {
		state = &parent.State
	}
	child, err = rootThreadRepairNodeTx(ctx, tx, child, state)
	if err != nil {
		return err
	}
	if err := writeThreadStateTx(ctx, tx, job.ID, child); err != nil {
		return err
	}
	return advanceThreadGraphCursorTx(ctx, tx, job.ID, child.ID)
}

func rootThreadRepairNodeTx(ctx context.Context, tx pgx.Tx, n threadRepairNode, parent *threadBoundaryState) (threadRepairNode, error) {
	n.State = n.State.rooted(parent)
	if n.ParentKey != "" || parent != nil {
		return n, nil
	}
	var opaque bool
	err := tx.QueryRow(ctx, `SELECT (source_system IS DISTINCT FROM 'email' AND NOT EXISTS(SELECT 1 FROM capture_import ci WHERE ci.activity_id=a.id)) OR EXISTS(SELECT 1 FROM activity_mail_reference r WHERE r.activity_id=a.id) FROM activity a WHERE a.id=@id`, pgx.NamedArgs{"id": n.ID}).Scan(&opaque)
	if err != nil {
		return n, err
	}
	if opaque {
		n.State.RootKey, err = opaqueThreadRootTx(ctx, tx, n)
		if err != nil {
			return n, err
		}
		n.State.RootProven = true
	}
	return n, nil
}

func optionalThreadParentTx(ctx context.Context, tx pgx.Tx, job ids.UUID, id *ids.ActivityID) (threadRepairNode, bool, error) {
	if id == nil {
		return threadRepairNode{}, false, nil
	}
	node, err := readThreadNodeTx(ctx, tx, job, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return threadRepairNode{}, false, nil
	}
	return node, err == nil, err
}
