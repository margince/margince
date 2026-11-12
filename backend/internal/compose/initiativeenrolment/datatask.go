// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiativeenrolment

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// Task dates the membership of every initiative that was live before the
// dated rows existed, through the writer the sweep runs. Those matches were
// never observed, so their dates are estimated.
func Task(store *initiatives.Store) datatask.Task {
	return datatask.Task{
		ID: "core/1794449739_initiative_enrolment", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794449739_initiative_enrolment"), Requires: []string{"core/1794449739"},
		Steps: []datatask.Step{{
			Name: "date_live_initiative_members", Limit: 20, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateUndated,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return dateBatch(ctx, tx, store, in)
			},
		}},
	}
}

func dateBatch(ctx context.Context, tx pgx.Tx, store *initiatives.Store, in datatask.BatchIn) (datatask.BatchOut, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := initiatives.LiveInitiativesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, id := range page {
		change, err := store.EstimateEnrolmentTx(ctx, tx, id)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if change.Joined+change.Left > 0 {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, page, changed)
}

func estimateUndated(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := initiatives.CountUndatedLiveInitiatives(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
