// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LockStopKey takes the transaction's lock over the stops of one key: a
// subject's uuid text, or an address no record holds. Every writer of
// communication_suppression for that key takes it first, so a stop recorded
// and an anonymize that pins the subject's stops never interleave. consent's
// writers and privacy's anonymize share this one spelling of the key.
func LockStopKey(ctx context.Context, tx pgx.Tx, key string) error {
	if key == "" {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key); err != nil {
		return fmt.Errorf("storekit: serialising stop writes for this subject: %w", err)
	}
	return nil
}
