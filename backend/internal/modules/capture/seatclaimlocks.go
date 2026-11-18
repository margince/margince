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
// connection needs: the label the row holds now and the label it will hold.
// A zero connection is a mailbox being added, which holds no label yet. Read
// before the row lock, as storekit.LockSeatAddressClaims requires.
func lockConnectionClaimsTx(ctx context.Context, tx pgx.Tx, user, connection ids.UUID, next *string) error {
	var current *string
	if !connection.IsZero() {
		err := tx.QueryRow(ctx, `SELECT account_label FROM capture_connection WHERE id = $1 AND user_id = $2`,
			connection, user).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("capture: reading the mailbox address a connection change touches: %w", err)
		}
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

// Ledger transitions hold the address check and sender decision locks before
// writing, so a reviewed verdict cannot race destruction of its sender's mail.
func lockPendingChecksTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var email string
	var owner ids.UUID
	err := tx.QueryRow(ctx, `SELECT email, owner_id FROM capture_pending_counterparty WHERE id = $1`, id).Scan(&email, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the address a ledger transition is about: %w", err)
	}
	if err := storekit.LockSeatAddressChecks(ctx, tx, email); err != nil {
		return err
	}
	return lockSenderDecisionTx(ctx, tx, owner, email)
}
