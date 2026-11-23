// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/winchance"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chances is the one win chance reader. It takes the assurance evidence, the
// forecast periods and the individual performance floor from this package.
var chances = winchance.New(winchance.Deps{
	RiskSubjects: func(ctx context.Context, tx pgx.Tx, at time.Time, only []ids.UUID) ([]assurance.Subject, assurance.Config, error) {
		subjects, err := assuranceEvidenceSubjects(ctx, tx, at, AssuranceCoverage(ctx, tx, at), only)
		return subjects, AssuranceConfig(), err
	},
	PeriodAt: func(ctx context.Context, tx pgx.Tx, kind forecasting.PeriodKind, at time.Time) (forecasting.Period, error) {
		period, _, err := ForecastPeriodAt(ctx, tx, kind, at)
		return period, err
	},
	NoLoneColleague: requireNoLoneColleague,
})

// WinChanceEngine is the win chance reader the composition wires everywhere.
func WinChanceEngine() *winchance.Engine { return chances }

// halvedProbabilityStore is a deal store with no win chance read, for the task
// that clears the retired halving.
func halvedProbabilityStore(pool *pgxpool.Pool) *deals.Store {
	inst := DealsInstallation()
	inst.WinChances, inst.WinChanceSQL = nil, nil
	return deals.NewStore(InstallationDB(pool), inst)
}
