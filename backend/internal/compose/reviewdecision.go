// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Closing the review a held message leaves behind, and finding the one that
// stands over it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviewCloser closes the review a settled message leaves behind.
//
// A seam rather than a direct call because activities may not import consent.
type reviewCloser struct{}

// CloseReviewForIntentTx implements activities.ReviewCloser.
//
// THE TRANSLATION LIVES HERE, at the seam, because each side owns half of it.
// activities knows what happened to the message and may not name consent's
// states; consent knows what the review row should say and has never heard of a
// scheduled send. Compose is the one place that has both.
func (reviewCloser) CloseReviewForIntentTx(
	ctx context.Context, tx pgx.Tx, intentID ids.UUID, outcome activities.ReviewOutcome,
) error {
	closure, known := reviewClosureFor(outcome)
	if !known {
		// A vocabulary this build does not understand, which can only mean
		// activities grew an outcome and this map was not extended. Refusing is
		// the safe direction: closing it as something else would write the
		// wrong account of why a message stopped, and
		// TestEveryReviewOutcomeHasAClosure keeps it from reaching a build.
		return fmt.Errorf(
			"compose: %q is not an outcome this build knows how to close a review for", outcome)
	}
	return consent.CloseReviewForIntentTx(ctx, tx, intentID, closure)
}

// reviewClosureFor maps one module's word for what happened onto the other's
// word for what the row should say.
func reviewClosureFor(outcome activities.ReviewOutcome) (consent.ReviewClosure, bool) {
	switch outcome {
	case activities.ReviewOutcomeCancelled:
		return consent.ClosedByCancellation(), true
	case activities.ReviewOutcomeSent:
		return consent.ClosedBySending(), true
	}
	return consent.ReviewClosure{}, false
}

// reviewLookup answers which review stands over a held message, so a scheduled
// row can offer a route to the work that would unstop it.
//
// A seam rather than a direct call because activities may not import consent.
// It carries the pool rather than a store because the read is one statement and
// needs nothing else.
type reviewLookup struct {
	pool *pgxpool.Pool
}

// LiveReviewsForIntents implements activities.ReviewLookup.
func (l reviewLookup) LiveReviewsForIntents(
	ctx context.Context, intents []ids.UUID,
) (map[ids.UUID]ids.UUID, error) {
	return consent.NewStore(InstallationDB(l.pool)).LiveReviewsForIntents(ctx, intents)
}
