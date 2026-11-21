// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"sync/atomic"

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

// boundAssuranceInputTypes is the set of event types the deal check reads from
// the outbox, nil while none is bound.
var boundAssuranceInputTypes atomic.Pointer[map[string]bool]

// UseAssuranceInputTypes binds the event types the deal check reads. An event
// of any other type is stored as already read, so it never enters the unread
// index. Unbound, every event starts unread, which is always correct.
func UseAssuranceInputTypes(types []string) {
	if types == nil {
		boundAssuranceInputTypes.Store(nil)
		return
	}
	set := make(map[string]bool, len(types))
	for _, eventType := range types {
		set[eventType] = true
	}
	boundAssuranceInputTypes.Store(&set)
}

// assuranceInputsRead is the assurance_inputs_read a new event of eventType
// starts with.
func assuranceInputsRead(eventType string) bool {
	set := boundAssuranceInputTypes.Load()
	return set != nil && !(*set)[eventType]
}
