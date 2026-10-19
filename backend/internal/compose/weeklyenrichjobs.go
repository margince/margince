// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/weekly"
)

func (w *weeklyGenerateWorker) enrichReview(ctx context.Context, review weekly.Review, now time.Time) {
	if review.NarratedAt == nil {
		w.narrate(ctx, review, now)
	}
	if review.LearningsState == weekly.LearningsNotRun {
		w.learn(ctx, review, now)
	}
}
