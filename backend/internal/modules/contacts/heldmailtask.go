// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RelabelHeldMailTx returns exactly the acquisitions whose duties the same transaction must settle.
func RelabelHeldMailTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) ([]ids.UUID, datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return nil, datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM contact_acquisition_evidence
 WHERE id > @after ORDER BY id LIMIT @limit FOR UPDATE`, pgx.NamedArgs{"after": after, "limit": in.Limit})
	if err != nil {
		return nil, datatask.BatchOut{}, err
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, datatask.BatchOut{}, err
	}
	rows, err = tx.Query(ctx, `UPDATE contact_acquisition_evidence e SET kind = 'mailbox_history'
 WHERE id = ANY(@ids) AND kind = 'unknown_legacy'
 AND (starts_with(e.captured_by, 'connector:') OR e.captured_by = 'agent:capture_counterparty_verdict')
 AND EXISTS (SELECT 1 FROM contact_email ce WHERE ce.contact_id = e.contact_id AND ce.archived_at IS NULL
 AND (`+sentHistorySQL+` OR `+receivedHistorySQL+`))
 RETURNING e.id`, pgx.NamedArgs{"ids": scanned, "expanded": false})
	if err != nil {
		return nil, datatask.BatchOut{}, err
	}
	changed, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, datatask.BatchOut{}, err
	}
	for _, id := range changed {
		if _, err := storekit.Audit(ctx, tx, "update", "contact_acquisition_evidence", id,
			map[string]any{"kind": AcquiredUnknownLegacy}, map[string]any{"kind": AcquiredMailboxHistory}); err != nil {
			return nil, datatask.BatchOut{}, err
		}
	}
	out, err := datatask.AdvanceUUID(in, scanned, len(changed))
	return changed, out, err
}

// EstimateHeldMailAcquisitions counts an indexed prefix of the rows the correction will inspect.
func EstimateHeldMailAcquisitions(ctx context.Context, q datatask.Querier, cap int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM
 (SELECT id FROM contact_acquisition_evidence ORDER BY id LIMIT @cap) AS candidates`,
		pgx.NamedArgs{"cap": cap + 1}).Scan(&count)
	return min(count, cap), count > cap, err
}
