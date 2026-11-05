// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (w *briefGenerateWorker) sendReadyWeekly(ctx context.Context, now time.Time) bool {
	engine := newWeeklyEngine(w.pool)
	review, err := currentMailReview(ctx, w.pool, engine, now)
	if err != nil {
		return false
	}
	pending, err := engine.MailPending(ctx, review.ID)
	if err != nil || !pending {
		return false
	}
	worker := weeklyGenerateWorker{engine: engine, pool: w.pool, users: w.users, log: w.log, mail: WeeklyMailConfig(w.mail), now: func() time.Time { return now }}
	worker.mailWeekly(ctx, review.ID, now)
	pending, err = engine.MailPending(ctx, review.ID)
	return err == nil && !pending
}

func routineCombines(settings identity.Delivery) bool {
	return settings.CombineRoutine != nil && *settings.CombineRoutine
}

func mailUser(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.Nil
	}
	return actor.UserID
}

func currentMailReview(ctx context.Context, pool *pgxpool.Pool, engine *weekly.Engine, now time.Time) (weekly.Review, error) {
	var week time.Time
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		week, err = weekly.WeekStartOf(ctx, tx, now)
		return err
	}); err != nil {
		return weekly.Review{}, err
	}
	previous := week.AddDate(0, 0, -7)
	return engine.LatestReview(ctx, &previous)
}
