// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// halvedProbabilitiesDataTask clears the probability the nightly close-date
// sweep used to write on quiet deals. The quiet effect on the win chance
// replaces it. The store carries no win chance read, because the clear echoes
// the deal and the task needs none.
func halvedProbabilitiesDataTask(pool *pgxpool.Pool) datatask.Task {
	inst := DealsInstallation()
	inst.WinChances, inst.WinChanceSQL = nil, nil
	store := deals.NewStore(InstallationDB(pool), inst)
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
