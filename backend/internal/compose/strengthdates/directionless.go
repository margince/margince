// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package strengthdates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DirectionlessSendersTask refolds the stored dates of each contact who wrote
// a message stored with no direction. Those dates were folded before such a
// message counted as theirs, and no row changes to fire a trigger.
func DirectionlessSendersTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795661340_directionless_sender_strength_dates", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795661340_directionless_sender_strength_dates"),
		Requires: []string{"core/1795661340"},
		Steps:    []datatask.Step{directionlessSendersStep()},
	}
}

// directionlessSendersStep walks contacts by id in bounded pages. A refold
// writes only dates that differ, so a second run changes no row.
func directionlessSendersStep() datatask.Step {
	return datatask.Step{
		Name: "contacts", Limit: 200, MinLimit: 10,
		Audit: datatask.AuditPolicy{Reason: "the strength-date triggers write the same derived columns without an audit"},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := storekit.CountClockRecords(ctx, q, storekit.ClockContacts, maximum)
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
			inspected, selected, err := contacts.DirectionlessSendersPageTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if err := contacts.RefreshStrengthDatesTx(ctx, tx, selected); err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, len(selected))
		},
	}
}
