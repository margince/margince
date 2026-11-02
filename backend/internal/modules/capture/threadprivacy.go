// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const threadSourceContentSQL = `a.archived_at IS NULL AND a.restricted_at IS NULL AND NOT (a.redacted_fields && ARRAY['subject','body']::text[])`

// Erasure locks activity before its caches; the inbox leaves repair locks to the worker.
func purgeThreadContentTx(ctx context.Context, tx pgx.Tx, activities []ids.UUID) error {
	args := pgx.NamedArgs{"activities": activities}
	rows, err := tx.Query(ctx, `SELECT id FROM activity WHERE id=ANY(@activities::uuid[]) ORDER BY id FOR UPDATE`, args)
	if err != nil {
		return err
	}
	if _, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID]); err != nil {
		return err
	}
	run, ok := principal.CorrelationID(ctx)
	if !ok {
		run = ids.NewV7()
	}
	args["run"] = run
	if _, err := tx.Exec(ctx, `INSERT INTO capture_thread_invalidation(activity_id,source_key,run_id)
 SELECT id,coalesce(thread_key,''),@run FROM activity WHERE id=ANY(@activities::uuid[]) AND kind='email'
 ON CONFLICT(activity_id) DO NOTHING`, args); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM capture_thread_topic WHERE activity_id=ANY(@activities::uuid[])`, args); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE capture_import SET thread_input_hash='' WHERE activity_id=ANY(@activities::uuid[])`, args); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `DELETE FROM capture_thread_reply WHERE activity_id=ANY(@activities::uuid[])`, args)
	return err
}

func threadSourceLiveTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity a WHERE a.id=@id AND `+threadSourceContentSQL+`
 AND NOT EXISTS(SELECT 1 FROM capture_thread_invalidation i WHERE i.activity_id=a.id))`, pgx.NamedArgs{"id": id}).Scan(&live)
	return live, err
}

func threadRepairInvalidationSQL(job string) string {
	return `EXISTS(SELECT 1 FROM capture_thread_invalidation WHERE repair_id=` + job + `
 UNION ALL SELECT 1 FROM capture_thread_barrier b JOIN capture_thread_invalidation i ON i.source_key=b.blocked_key WHERE b.repair_id=` + job + `
 UNION ALL SELECT 1 FROM capture_thread_repair_node n JOIN capture_thread_invalidation i ON i.activity_id=n.activity_id WHERE n.repair_id=` + job + `)`
}

func threadRepairInvalidatedTx(ctx context.Context, tx pgx.Tx, job ids.UUID) (bool, error) {
	var invalid bool
	err := tx.QueryRow(ctx, `SELECT `+threadRepairInvalidationSQL("@job"), pgx.NamedArgs{threadArgJob: job}).Scan(&invalid)
	return invalid, err
}
