// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/assurancefloor"
	"github.com/margince/margince/backend/internal/modules/assurance"
)

// assuranceRun is the one floored read of the latest run, which the route and
// the forecast_input_checks tool both answer from.
func assuranceRun(pool *pgxpool.Pool) assurance.RunFunc {
	store := assurance.NewStore(InstallationDB(pool))
	return func(ctx context.Context) (assurance.Run, []assurance.SourceCoverage, error) {
		return assurancefloor.ReadRun(ctx, pool, store, requireNoLoneColleague)
	}
}
