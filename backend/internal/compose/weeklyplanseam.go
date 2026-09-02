// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one edge between the week just gone and the week ahead.
//
// compose/weekly owns weekly_review and weeklyplan owns the plan tables, and
// neither may write the other's. So the retrospective asks the plan what its
// week came to and writes the answer into its own row — two integers across the
// seam, and the plan's rows stay the plan's business.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
)

type weeklyPlanOutcome struct{ store *weeklyplan.Store }

var _ weekly.WeekPlan = weeklyPlanOutcome{}

func (o weeklyPlanOutcome) CloseWeek(ctx context.Context, now time.Time) (int, int, error) {
	outcome, err := o.store.CloseWeek(ctx, now)
	if err != nil {
		return 0, 0, err
	}
	return outcome.Due, outcome.Kept, nil
}

// weeklyPlanStore builds the plan store.
//
// ONE spelling of "which Monday": the plan and the review beside it must be
// about the same seven days, and compose/weekly owns that answer. A module may
// not import compose, so it takes the function.
func weeklyPlanStore(pool *pgxpool.Pool) *weeklyplan.Store {
	return weeklyplan.NewStore(InstallationDB(pool), weekly.WeekStartOf, newTeammatesSeam(pool))
}
