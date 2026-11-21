// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package strengthdates folds the stored strength dates of every live contact
// that existed before the dates were stored. Until a contact is folded its
// strength reads fold its history, so the task changes no number a user sees.
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

// DataTask is the backfill. An archived contact stores no dates, so it is left.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795264358_contact_strength_dates", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795264358_contact_strength_dates"),
		Requires: []string{"core/1795264358"},
		Steps:    []datatask.Step{foldStep()},
	}
}

// foldStep walks contacts by id in bounded pages and folds the unfolded ones.
// A folded contact leaves the selection, so a second run changes nothing.
func foldStep() datatask.Step {
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
			inspected, selected, err := storekit.ClockPage(ctx, tx, storekit.ClockContacts, storekit.ClockStrengthUnfolded, after, in.Limit)
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
