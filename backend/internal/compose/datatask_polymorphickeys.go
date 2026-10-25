// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// polymorphicKeysDataTask keys the signals, weekly commitments and record views
// written before their per-type keys, so a deleted record reaches them too. A
// row whose record is already gone ends as its key would end it. A signal or
// view is deleted, and a commitment lets go of the link.
func polymorphicKeysDataTask() datatask.Task {
	// Each step does what the key's ON DELETE action does, which writes no
	// audit row either.
	follows := func(action string) datatask.AuditPolicy {
		return datatask.AuditPolicy{Reason: "the trigger keys every row written today, and for a gone record the key's ON DELETE " +
			action + " does the same without an audit row"}
	}
	return datatask.Task{
		ID: "core/1792893103_polymorphic_rows_keyed", Version: 1,
		Title:    "Linking signals, weekly commitments and record views to the records they name",
		Requires: []string{"core/1792893103"},
		Steps: []datatask.Step{
			keyPolymorphicRowsStep("key_signals", follows("CASCADE"), signals.KeyEntitiesTx, signals.EstimateUnkeyedEntities),
			keyPolymorphicRowsStep("key_weekly_commitments", follows("SET NULL"),
				weeklyplan.KeyLinkedRecordsTx, weeklyplan.EstimateUnkeyedLinks),
			keyPolymorphicRowsStep("key_record_views", follows("CASCADE"), company360.KeyVisitsTx, company360.EstimateUnkeyedVisits),
		},
	}
}

// keyPolymorphicRowsStep pages one table's unkeyed rows through its owner's
// writer. Every inspected row is changed, so the page is also the change count.
func keyPolymorphicRowsStep(
	name string, audit datatask.AuditPolicy,
	key func(context.Context, pgx.Tx, ids.UUID, int) ([]ids.UUID, error),
	estimate func(context.Context, interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}, int64) (int64, error),
) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 500, MinLimit: 10, Audit: audit,
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := estimate(ctx, q, maximum)
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
			changed, err := key(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, changed, len(changed))
		},
	}
}
