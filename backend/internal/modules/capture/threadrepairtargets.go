// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func discoverThreadTargetUnitTx(ctx context.Context, tx pgx.Tx, job threadRepair) error {
	var id ids.ActivityID
	var key string
	err := tx.QueryRow(ctx, `SELECT activity_id,root_key FROM capture_thread_repair_node WHERE repair_id=@job AND activity_id>@after ORDER BY activity_id LIMIT 1`, pgx.NamedArgs{threadArgJob: job.ID, threadArgAfter: job.After}).Scan(&id, &key)
	if errors.Is(err, pgx.ErrNoRows) {
		return threadRepairPhaseTx(ctx, tx, job.ID, threadPhaseInherit)
	}
	if err != nil {
		return err
	}
	if err := addThreadBarrierTx(ctx, tx, job.ID, key); err != nil {
		return err
	}
	var unloaded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_thread_barrier WHERE repair_id=@job AND NOT loaded)`, pgx.NamedArgs{threadArgJob: job.ID}).Scan(&unloaded); err != nil {
		return err
	}
	if unloaded {
		return threadRepairPhaseTx(ctx, tx, job.ID, "load")
	}
	return advanceThreadNodeTx(ctx, tx, job.ID, id)
}

func opaqueThreadRootTx(ctx context.Context, tx pgx.Tx, node threadRepairNode) (string, error) {
	var collides bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity_identity i WHERE i.identity_kind='mail' AND i.identity_key=@source AND i.activity_id<>@id) OR EXISTS(SELECT 1 FROM activity a WHERE a.source_system='email' AND a.source_id=@source AND a.id<>@id)`, pgx.NamedArgs{threadArgSource: node.Source, "id": node.ID}).Scan(&collides)
	if err != nil || !collides {
		return node.Source, err
	}
	var first ids.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM activity WHERE thread_key=@source AND kind='email' AND source_system IS DISTINCT FROM 'email' ORDER BY id LIMIT 1`, pgx.NamedArgs{threadArgSource: node.Source}).Scan(&first)
	if err != nil {
		return "", err
	}
	return "thread:opaque:" + first.String(), nil
}
