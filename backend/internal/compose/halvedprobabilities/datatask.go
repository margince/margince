// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package halvedprobabilities holds the data task that clears the probability
// the nightly close-date sweep used to write on quiet deals.
package halvedprobabilities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask clears what the sweep wrote, through the given deal store. The quiet
// effect on the win chance replaces that write. The store should carry no win
// chance read, because the clear echoes the deal and the task needs no chance.
func DataTask(store *deals.Store) datatask.Task {
	return datatask.Task{ID: "core/1795355731_retire_halved_probabilities", Version: 1, Title: systemtext.Literal("datatask.title.core.1795355731_retire_halved_probabilities"), Steps: []datatask.Step{{
		Name: "retire_halved_probabilities", Limit: 25, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: deals.CountHalvedProbabilityCandidates,
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			scanned, cleared, err := store.RetireHalvedProbabilitiesTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, scanned, cleared)
		},
	}}}
}
