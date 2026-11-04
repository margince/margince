// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// heldContactsDataTask releases contacts whose held thread cleared before the
// runtime release existed. It pages every contact by primary key and runs that
// same release on each held one. A contact some hold still covers stays held.
func heldContactsDataTask(pool *pgxpool.Pool) datatask.Task {
	return datatask.Task{
		ID: "core/1793766966_release_ended_contact_holds", Version: 1,
		Title: "Releasing contacts whose confidentiality hold ended", Requires: []string{"core/1793766965"},
		Steps: []datatask.Step{{
			Name: "release_ended_contact_holds", Limit: 50, MinLimit: 5,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateHeldContacts, Batch: releaseHeldContactsBatch(newCounterpartyStore(pool)),
		}},
	}
}

func releaseHeldContactsBatch(store *contacts.Store) func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
	return func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		if err := auth.RequireSystem(ctx); err != nil {
			return datatask.BatchOut{}, err
		}
		after, err := datatask.UUIDCursor(in.Cursor)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		page, err := contacts.ConfidentialityHeldPageTx(ctx, tx, after, in.Limit)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		changed := 0
		for _, id := range page.Held {
			out, err := releaseHeldContactTx(ctx, tx, store, id)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			if out == releasePublished || out == releaseRenarrowed {
				changed++
			}
		}
		return datatask.AdvanceUUID(in, page.Scanned, changed)
	}
}

func estimateHeldContacts(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountContactsToInspect(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
