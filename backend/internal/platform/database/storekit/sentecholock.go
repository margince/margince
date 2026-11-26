// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LockSentEchoKey serialises the two sides that learn a sent message's
// provider-stamped identity. The receipt looks for an echo already captured;
// the echo's capture looks for the receipt. Whichever locks second sees what
// the first committed. comms and capture share this one spelling of the key.
func LockSentEchoKey(ctx context.Context, tx pgx.Tx, seat ids.UUID, provider, providerMessageID string) error {
	if providerMessageID == "" {
		return nil
	}
	key := "sent_echo:" + seat.String() + ":" + provider + ":" + providerMessageID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("storekit: serialising the sent message's echo: %w", err)
	}
	return nil
}
