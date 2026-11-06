// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordAssuranceInputsRead records events covered by a complete check or its
// freshness consumer, in the transaction that records what was read.
func RecordAssuranceInputsRead(ctx context.Context, tx pgx.Tx, eventIDs []ids.UUID) error {
	if len(eventIDs) == 0 {
		return nil
	}
	keys := make([]string, len(eventIDs))
	for i, id := range eventIDs {
		keys[i] = id.String()
	}
	_, err := tx.Exec(ctx, `UPDATE event_outbox SET assurance_inputs_read = true
		WHERE NOT assurance_inputs_read AND envelope->>'event_id' = ANY(@events::text[])`, pgx.NamedArgs{"events": keys})
	if err != nil {
		return fmt.Errorf("store: recording the input events the check read: %w", err)
	}
	return nil
}
