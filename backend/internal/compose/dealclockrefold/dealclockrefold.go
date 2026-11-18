// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dealclockrefold is the data task that refolds the stored deal clocks
// written before a deal counted the mail only its participants may read.
package dealclockrefold

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask walks the deals by id and refolds each stored clock that no longer
// matches its folded value. A refolded deal leaves the selection.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795014819_private_mail_moves_the_deal_clock", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795014819_private_mail_moves_the_deal_clock"),
		Requires: []string{"core/1795014819"},
		Steps: []datatask.Step{{
			Name: "deals", Limit: 200, MinLimit: 10,
			Audit: datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				count, err := storekit.CountClockRecords(ctx, q, storekit.ClockDeals, maximum)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				if err := auth.RequireSystem(ctx); err != nil {
					return datatask.BatchOut{}, err
				}
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				inspected, selected, err := storekit.ClockPage(ctx, tx, storekit.ClockDeals, storekit.ClockNotFolded, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				targets := storekit.LastActivityTargets{Deals: selected}
				if err := storekit.LockLastActivityTargets(ctx, tx, targets); err != nil {
					return datatask.BatchOut{}, err
				}
				if err := deals.RefoldClocksTx(ctx, tx, selected); err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, inspected, len(selected))
			},
		}},
	}
}
