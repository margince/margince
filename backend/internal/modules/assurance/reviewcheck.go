// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReviewCheckFunc confirms an answer to a review against the deal it is about,
// inside the answer's transaction. Compose supplies it, because the evidence
// lives in modules this one does not import.
type ReviewCheckFunc func(ctx context.Context, tx pgx.Tx, deal ids.UUID, outcome string, now time.Time) error

// WithReviewCheck binds the review check.
func (s *Store) WithReviewCheck(check ReviewCheckFunc) *Store {
	wired := *s
	wired.reviewCheck = check
	return &wired
}

// applyReviewCheck runs the review check when the answered finding is a review.
// Unwired, a review answer that claims the deal is back or gone is refused:
// nothing confirmed it.
func (s *Store) applyReviewCheck(ctx context.Context, tx pgx.Tx, exception ids.UUID, outcome string, now time.Time) error {
	var kind string
	var deal ids.UUID
	if err := tx.QueryRow(ctx, `SELECT type, subject_id FROM assurance_exception WHERE id = $1`,
		exception).Scan(&kind, &deal); err != nil {
		return fmt.Errorf("assurance: reading the answered finding: %w", err)
	}
	if !slices.Contains(ReviewTypes(), kind) {
		return nil
	}
	if s.reviewCheck == nil {
		if outcome == OutcomeFixedRecord || outcome == OutcomeNotRelevant {
			return fmt.Errorf("assurance: the review check is not wired")
		}
		return nil
	}
	return s.reviewCheck(ctx, tx, deal, outcome, now)
}
