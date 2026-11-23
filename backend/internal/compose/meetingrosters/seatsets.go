// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingrosters

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// SeatSetsDataTask moves the one roster seat of an older guest row into the
// row's seat set. Readers take both columns as one set, so a row is right
// before the task reaches it; the task leaves one column to read.
func SeatSetsDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795336082_guest_rows_name_every_calendar", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795336082_guest_rows_name_every_calendar"),
		Requires: []string{"core/1795346515"},
		Steps: []datatask.Step{{
			Name: "copy_roster_seats", Limit: 500, MinLimit: 20, EmptyWhenDone: true,
			Audit: datatask.AuditPolicy{Reason: "capture's roster writers store the same seat set on the rows they write, without a separate audit"},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				total, err := activities.CountOneSeatRosterRows(ctx, q, maximum)
				return min(total, maximum), total > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				inspected, changed, err := activities.CopyRosterSeatsTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, inspected, changed)
			},
		}},
	}
}
