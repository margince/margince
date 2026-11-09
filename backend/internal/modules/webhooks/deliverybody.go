// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// bodyStillCurrent serializes the last send check with retention and erasure.
// The transaction ends before the network call.
func (s *Store) bodyStillCurrent(ctx context.Context, target attemptTarget) (bool, error) {
	var current bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT payload = @body FROM webhook_delivery
   WHERE id = @id AND status IN ('pending', 'retrying')
     AND payload_pruned_at IS NULL
     AND claimed_until IS NOT DISTINCT FROM @claim
   FOR SHARE`, pgx.NamedArgs{"id": target.deliveryID, "claim": target.claim, "body": string(target.payload)}).Scan(&current)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return current, err
}

func (d *Deliverer) sendAuthorized(ctx context.Context, target attemptTarget) bool {
	if d.beforeSend != nil {
		if err := d.beforeSend(ctx); err != nil {
			d.spendUncheckedAttempt(ctx, target, err)
			return false
		}
	}
	current, err := d.store.bodyStillCurrent(ctx, target)
	if err != nil {
		d.spendUncheckedAttempt(ctx, target, err)
	}
	return current && err == nil
}
