// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lockConnectionClaimsTx takes the seat-address claim locks a change to one
// connection needs: the label it holds now and the label it will hold. Read
// before the row lock, as storekit.LockSeatAddressClaims requires.
func lockConnectionClaimsTx(ctx context.Context, tx pgx.Tx, user ids.UUID, provider string, next *string) error {
	var current *string
	err := tx.QueryRow(ctx, `SELECT account_label FROM capture_connection WHERE user_id = $1 AND provider = $2`,
		user, provider).Scan(&current)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("capture: reading the mailbox address a connection change touches: %w", err)
	}
	var labels []string
	for _, label := range []*string{current, next} {
		if label != nil {
			labels = append(labels, *label)
		}
	}
	return storekit.LockSeatAddressClaims(ctx, tx, labels...)
}

// lockIdentityClaimTx takes the claim lock of the address an identity row
// holds, before the row is removed.
func lockIdentityClaimTx(ctx context.Context, tx pgx.Tx, id, user ids.UUID) error {
	var value string
	err := tx.QueryRow(ctx, `
		SELECT value FROM capture_owner_identity WHERE id = $1 AND user_id = $2 AND kind = $3`,
		id, user, IdentityKindAddress).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the address a removal withdraws: %w", err)
	}
	return storekit.LockSeatAddressClaims(ctx, tx, value)
}

// lockPendingChecksTx takes the check lock of the address a ledger row asks
// about, before a transition writes the row (contacts/lockorder.go).
func lockPendingChecksTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var email string
	err := tx.QueryRow(ctx, `SELECT email FROM capture_pending_counterparty WHERE id = $1`, id).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the address a ledger transition is about: %w", err)
	}
	return storekit.LockSeatAddressChecks(ctx, tx, email)
}
