// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
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
	return s.legacyLeadOccurrenceOwned(ctx, lead, LegacyLeadHandlerNames(), false)
}

// Submission identity survives promotion, merging and task closure.
func (s *AutomationStore) LegacyLeadTaskOwned(ctx context.Context, submission string) (bool, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return false, err
	}
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return false, err
	}
	var lead ids.UUID
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM lead WHERE source_system=@source AND source_id=@submission`,
			pgx.NamedArgs{"source": provenance.WebFormSource, "submission": submission}).Scan(&lead)
		if err != nil {
			return err
		}
		return auth.EnsureVisible(ctx, tx, "lead", lead)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return s.legacyLeadOccurrenceOwned(ctx, lead, []string{routeLeadName}, true)
}

func (s *AutomationStore) legacyLeadOccurrenceOwned(ctx context.Context, lead ids.UUID, handlers []string, taskOnly bool) (bool, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return false, err
	}
	var owned bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
 SELECT 1 FROM workflow_run r LEFT JOIN event_outbox e ON (`+kevents.OutboxEventIDSQL("e")+`)=r.trigger_event::text
 WHERE r.handler=ANY(@handlers) AND r.status<>'skipped'
 AND ((e.envelope->>'type'=@event AND e.envelope->'entity'->>'id'=@lead)
 OR EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(r.planned)='array' THEN r.planned ELSE '[]'::jsonb END) action
 WHERE action->'Target'->>'Type'='lead' AND action->'Target'->>'ID'=@lead))
 AND (NOT @task_only OR EXISTS(SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(r.planned)='array' THEN r.planned ELSE '[]'::jsonb END) action
 WHERE action->>'Kind'=@kind AND action->'Target'->>'Type'='lead' AND action->'Target'->>'ID'=@lead))
)`, pgx.NamedArgs{"handlers": handlers, "event": eventLeadCreated, "lead": lead.String(), "task_only": taskOnly, "kind": string(workflow.ActionCreateTask)}).Scan(&owned)
	})
	return owned, err
}
