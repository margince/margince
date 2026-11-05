// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const noticeKindArg = "kind"

// PreviewNoticeCarryTx refuses to expose another recipient's notifications during merge review.
func PreviewNoticeCarryTx(ctx context.Context, tx pgx.Tx, kind string, source ids.UUID) ([]crmcontracts.NotificationItem, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT recipient_user_id FROM notice WHERE target_type=@kind AND target_id=@source LIMIT 2`, pgx.StrictNamedArgs{noticeKindArg: kind, "source": source})
	if err != nil {
		return nil, err
	}
	recipients, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		return []crmcontracts.NotificationItem{}, nil
	}
	seat, err := actingSeat(ctx, "reviewing notifications moved by a merge")
	if err != nil {
		return nil, err
	}
	// A merge changes every recipient's notice; a partial view would conceal its scope.
	for _, recipient := range recipients {
		if recipient != seat {
			return nil, apperrors.ErrNotFound
		}
	}
	rows, err = tx.Query(ctx, `SELECT `+centreColumns+`
 FROM notice WHERE target_type=@kind AND target_id=@source ORDER BY id LIMIT 501`, pgx.StrictNamedArgs{noticeKindArg: kind, "source": source})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanCentreItems(rows)
	if err != nil {
		return nil, err
	}
	if len(items) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make([]crmcontracts.NotificationItem, 0, len(items))
	for _, item := range items {
		out = append(out, centreItemWire(item))
	}
	return out, nil
}
