// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What MOVES when two rows turn out to be one message.
//
// The absorb splits its work in two: evidence is COPIED, so the surviving row
// gains what the folded-in one held without either losing it, and work items
// are MOVED, because a queued question asked twice is asked twice. These are
// the movers.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// repointEchoReviews MOVES the echo's queued counterparty dispositions onto the
// survivor — the one thing here that is moved rather than copied, because it is
// a work item and not evidence. What those rows hold is a queued HUMAN review,
// and an ensure-retry cursor, that the survivor does not re-queue: left on the
// row this absorb archives, the question "who is this stranger?" would be asked
// about a message the workspace can no longer see, and copied it would be asked
// twice. Their live-row uniqueness keys on (email), which this
// write does not touch, so a re-point can collide with nothing.
func repointEchoReviews(ctx context.Context, tx pgx.Tx, survivorID, echoID ids.ActivityID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE capture_pending_counterparty SET activity_id = $1 WHERE activity_id = $2`,
		survivorID, echoID); err != nil {
		return fmt.Errorf("activities: re-pointing the absorbed echo's queued counterparty reviews: %w", err)
	}
	return nil
}
