// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/platform/database"
)

// The same opening hour governs personal and team measurements. An empty
// candidate list alone cannot distinguish a finished pass from a premature one.
func (w *weeklyGenerateWorker) reviewWindowOpen(ctx context.Context, now time.Time) (bool, error) {
	var ready bool
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		overlay, err := overlayModeOf(ctx, tx)
		if err != nil {
			return err
		}
		if overlay {
			w.log.InfoContext(ctx, "weekly review skipped: CRM measurement belongs to the incumbent")
			return nil
		}
		_, local, err := briefs.LocalDayAt(ctx, tx, now)
		if err != nil {
			return err
		}
		ready = local.Weekday() != time.Monday || local.Hour() >= reviewHour
		return nil
	})
	return ready, err
}

func (w *weeklyGenerateWorker) enrichReview(ctx context.Context, review weekly.Review, now time.Time) {
	if review.NarratedAt == nil {
		w.narrate(ctx, review, now)
	}
	if review.LearningsState == weekly.LearningsNotRun {
		w.learn(ctx, review, now)
	}
}
