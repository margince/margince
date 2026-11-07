// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type inputPosition struct {
	generation int64
	events     []ids.UUID
}

func markingEventTypes() []string {
	var out []string
	for _, name := range DecidedInputEvents() {
		change, ok := InputChangeOf(name)
		if ok && change.Scope != ScopeNone {
			out = append(out, name)
		}
	}
	return out
}

func unreadInputEvents(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT (envelope->>'event_id')::uuid FROM event_outbox
		WHERE NOT assurance_inputs_read AND envelope->>'type' = ANY(@types::text[])`,
		pgx.NamedArgs{"types": markingEventTypes()})
	if err != nil {
		return nil, fmt.Errorf("assurance: reading committed input events: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

func inputsAwaitReading(ctx context.Context, tx pgx.Tx) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM event_outbox
		WHERE NOT assurance_inputs_read AND envelope->>'type' = ANY(@types::text[]))`,
		pgx.NamedArgs{"types": markingEventTypes()}).Scan(&pending)
	if err != nil {
		return false, fmt.Errorf("assurance: checking for unread committed inputs: %w", err)
	}
	return pending, nil
}
