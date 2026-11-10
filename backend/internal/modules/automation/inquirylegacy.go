// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func LegacyLeadHandlerNames() []string {
	var names []string
	for _, entry := range Catalog() {
		if entry.Trigger == eventLeadCreated {
			names = append(names, entry.Key)
		}
	}
	return names
}

// LegacyLeadOccurrenceOwned preserves work claimed before its website note could commit.
func (s *AutomationStore) LegacyLeadOccurrenceOwned(ctx context.Context, lead ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return false, err
	}
	var owned bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
 SELECT 1 FROM workflow_run r JOIN event_outbox e ON (`+kevents.OutboxEventIDSQL("e")+`)=r.trigger_event::text
 WHERE r.handler=ANY(@handlers) AND r.status<>'skipped'
 AND e.envelope->>'type'=@event AND e.envelope->'entity'->>'id'=@lead
)`, pgx.NamedArgs{"handlers": LegacyLeadHandlerNames(), "event": eventLeadCreated, "lead": lead.String()}).Scan(&owned)
	})
	return owned, err
}
