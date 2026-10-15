// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// claimLease bounds a claim. It outlasts one attempt, so a live attempt keeps
// its row. It is short, so a crashed worker's row is retried soon.
const claimLease = 2 * deliveryTimeout

// errDeliveryInFlight is a replay of a delivery that a sweep or another replay
// is sending now. It maps to a 409.
var errDeliveryInFlight = fmt.Errorf("this delivery is being sent now; wait for the attempt to finish: %w", apperrors.ErrConflict)

// checkReplayable refuses a replay the delivery's state does not allow. A
// render failure is parked before any attempt with nothing stored, while an
// empty body that was rendered and sent replays like any other.
func checkReplayable(ctx context.Context, tx pgx.Tx, subID, deliveryID ids.UUID) error {
	var status string
	var renderFailed, inFlight bool
	err := tx.QueryRow(ctx, `
		SELECT status, status = 'dead_lettered' AND attempts = 0 AND payload = '',
		       claimed_until IS NOT NULL AND claimed_until >= now()
		FROM webhook_delivery WHERE id = $1 AND subscription_id = $2`,
		deliveryID, subID).Scan(&status, &renderFailed, &inFlight)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.ErrNotFound
	case err != nil:
		return err
	case renderFailed:
		return &BadInputError{Field: "delivery", Reason: "this delivery has no body to resend because its template could not be rendered; fix the template and the next event will use it"}
	// A revoked row goes on to the visibility re-check, which keeps it revoked.
	case status != "dead_lettered" && status != "retrying" && status != "visibility_revoked":
		return &BadInputError{Field: "delivery", Reason: fmt.Sprintf("a %s delivery cannot be replayed; only a dead-lettered or retrying one can", status)}
	case inFlight:
		return errDeliveryInFlight
	}
	return nil
}
