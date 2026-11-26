// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package strengthdates folds the stored strength dates of every live contact
// whose dates are not folded. Those are the contacts that existed before the
// dates were stored, and every contact after a move of the installation zone.
// Until a contact is folded its strength reads fold its history, so no number
// a user sees changes.
package strengthdates

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask is the backfill. An archived contact stores no dates, so it is left.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795269930_contact_strength_dates", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795269930_contact_strength_dates"),
		Requires: []string{"core/1795299767"},
		Steps:    []datatask.Step{refoldStep(unfoldedPage)},
	}
}

// contactPage reads the next limit contacts after `after`, in id order. It
// answers every id it inspected, for the cursor, and the ones to refold.
type contactPage func(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (inspected, selected []ids.UUID, err error)

// unfoldedPage selects the contacts whose dates were never folded. A folded
// contact leaves the selection, so a second run changes nothing.
func unfoldedPage(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (inspected, selected []ids.UUID, err error) {
	return storekit.ClockPage(ctx, tx, storekit.ClockContacts, storekit.ClockStrengthUnfolded, after, limit)
}

// refoldStep walks contacts by id in bounded pages and refolds the stored
// dates of the ones page selects.
func refoldStep(page contactPage) datatask.Step {
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
			inspected, selected, err := page(ctx, tx, after, in.Limit)
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

// foldBatch and foldBatches bound one pass of the clock refold job. That folds
// an installation within hours of a zone move, without holding the job.
const (
	foldBatch   = 200
	foldBatches = 10
)

// FoldUnfolded folds up to foldBatches batches of unfolded live contacts, one
// transaction each, and answers how many it folded.
func FoldUnfolded(ctx context.Context, db *database.DB) (int, error) {
	folded := 0
	for range foldBatches {
		var batch []ids.UUID
		if err := db.Tx(ctx, func(tx pgx.Tx) (err error) {
			if batch, err = contacts.UnfoldedContactsTx(ctx, tx, foldBatch); err != nil {
				return err
			}
			return contacts.RefreshStrengthDatesTx(ctx, tx, batch)
		}); err != nil {
			return folded, err
		}
		folded += len(batch)
		if len(batch) < foldBatch {
			break
		}
	}
	return folded, nil
}
