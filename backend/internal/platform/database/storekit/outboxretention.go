// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OutboxCopy is one published event and the record its envelope names. The
// record id is empty when the envelope names none.
type OutboxCopy struct {
	ID         ids.UUID
	EntityType string
	EntityID   string
}

// OutboxReaders are the modules that still read published events. The
// assurance check needs an event of a type in UnreadTypes until it has read it.
// Each of NeededBy is true when its module still needs the event aliased by its
// argument. Modules sit above storekit, so compose supplies both.
type OutboxReaders struct {
	UnreadTypes []string
	NeededBy    []func(alias string) string
}

// PublishedOutboxCopies is up to limit events after the cursor that were
// published before cutoff and that no reader of the outbox still needs. Besides
// readers, the intake rule needs the event an imported follow-up was caused by.
// An unpublished event is never answered.
func PublishedOutboxCopies(ctx context.Context, tx pgx.Tx, cutoff time.Time, after ids.UUID, limit int, readers OutboxReaders) ([]OutboxCopy, error) {
	var needed strings.Builder
	for _, reader := range readers.NeededBy {
		needed.WriteString("\n\t\t  AND NOT " + reader("o"))
	}
	rows, err := tx.Query(ctx, `SELECT o.id, coalesce(o.envelope->'entity'->>'type', ''), coalesce(o.envelope->'entity'->>'id', '')
		FROM event_outbox o
		WHERE o.published_at IS NOT NULL AND o.published_at < @cutoff AND o.id > @after
		  AND NOT (NOT o.assurance_inputs_read AND o.envelope->>'type' = ANY(@unread::text[]))
		  AND NOT `+auth.ImportedFollowupEvidenceSQL("o")+needed.String()+`
		ORDER BY o.id LIMIT @limit`,
		pgx.StrictNamedArgs{"cutoff": cutoff, "after": after, "unread": readers.UnreadTypes, pageLimitParameter: limit})
	if err != nil {
		return nil, fmt.Errorf("store: reading published events past their window: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[OutboxCopy])
}

// DeleteOutboxCopies removes published events. An event the relay has not
// published yet is never removed, whatever the caller passed.
func DeleteOutboxCopies(ctx context.Context, tx pgx.Tx, events []ids.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM event_outbox WHERE id = ANY(@events) AND published_at IS NOT NULL`,
		pgx.StrictNamedArgs{"events": events}); err != nil {
		return fmt.Errorf("store: removing published events past their window: %w", err)
	}
	return nil
}
