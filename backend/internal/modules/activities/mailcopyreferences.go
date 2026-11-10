// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Every incoming foreign key participates, including references added after the repair shipped.
func mailCopyReferencedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT t.relname,a.attname FROM pg_constraint c JOIN pg_class t ON t.oid=c.conrelid JOIN pg_namespace n ON n.oid=t.relnamespace JOIN LATERAL unnest(c.conkey,c.confkey) key(local_number,target_number) ON true JOIN pg_attribute a ON a.attrelid=t.oid AND a.attnum=key.local_number JOIN pg_attribute target ON target.attrelid=c.confrelid AND target.attnum=key.target_number WHERE c.contype='f' AND target.attname='id' AND c.confrelid='activity'::regclass AND n.nspname=current_schema()`)
	if err != nil {
		return false, err
	}
	type reference struct{ table, column string }
	refs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (reference, error) {
		var one reference
		err := row.Scan(&one.table, &one.column)
		return one, err
	})
	if err != nil {
		return false, err
	}
	for _, ref := range refs {
		// These rows move with the fold, or are the discarded headers; the mailbox remains authoritative.
		if ref.column == fieldMeetingID && (ref.table == "activity_link" || ref.table == "activity_identity" || ref.table == "activity_reader_state" || ref.table == "capture_import" || ref.table == "attachment" || ref.table == "activity_participant") {
			continue
		}
		var exists bool
		args := []any{id}
		query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s WHERE %s=$%d)`, pgx.Identifier{ref.table}.Sanitize(), pgx.Identifier{ref.column}.Sanitize(), len(args))
		if err := tx.QueryRow(ctx, query, args...).Scan(&exists); err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}
