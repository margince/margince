// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func contactEmployersDataTask(pool *pgxpool.Pool) datatask.Task {
	store := newContactsStore(InstallationDB(pool)).WithConsumerMail(capture.MatcherTx)
	return datatask.Task{
		ID: "core/1793260800_contact_employers", Version: 1,
		Title: "Resolving employers for existing contacts", Requires: []string{dedupeScanRequiredMigration},
		Steps: []datatask.Step{{
			Name: "contact_employers", Limit: 50, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateContactEmployers,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				if err := auth.RequireSystem(ctx); err != nil {
					return datatask.BatchOut{}, err
				}
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				page, err := store.ReconcileContactEmployersTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
			},
		}},
	}
}

func estimateContactEmployers(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountMissingContactEmployers(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
