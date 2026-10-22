// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TaskClaim retains recipient identity independently of task completion.
type TaskClaim struct {
	ClaimOnTask
	Task        *ids.UUID
	Locator     string
	DisplayName string
	Status      string
}

// ClaimsOnTasks applies both task and contact visibility before returning recipients.
func (s *Store) ClaimsOnTasks(ctx context.Context, tasks []ids.UUID) ([]TaskClaim, error) {
	return s.readTaskClaims(ctx, tasks, "c.task_activity_id")
}

// ClaimsByIDs preserves the same visibility rules when a proposal names recipients.
func (s *Store) ClaimsByIDs(ctx context.Context, claims []ids.UUID) ([]TaskClaim, error) {
	return s.readTaskClaims(ctx, claims, "c.id")
}

func (s *Store) readTaskClaims(ctx context.Context, named []ids.UUID, identity string) ([]TaskClaim, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(named) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	taskPos := arg(named)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	evidence, err := claimContentClause(ctx, arg)
	if err != nil {
		return nil, err
	}
	taskScope, err := auth.ActivityContentClause(ctx, "task", arg)
	if err != nil {
		return nil, err
	}
	var out []TaskClaim
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, fmt.Sprintf(`
 SELECT c.id, c.contact_id, c.source_activity_id, c.task_activity_id, pr.full_name, c.status, c.evidence_fingerprint
 FROM conversation_claim c
 JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
 JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
 LEFT JOIN activity task ON task.id = c.task_activity_id AND task.archived_at IS NULL
 WHERE %s = ANY($%d) AND c.archived_at IS NULL
 AND (%s) AND (%s) AND (c.task_activity_id IS NULL OR (task.id IS NOT NULL AND (%s)))
 ORDER BY c.task_activity_id, pr.full_name, c.id`, identity, taskPos, scope, evidence, taskScope), args...)
		if err != nil {
			return fmt.Errorf("read task recipient claims: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskClaim, error) {
			var claim TaskClaim
			err := row.Scan(&claim.ID, &claim.Contact, &claim.Source, &claim.Task, &claim.DisplayName, &claim.Status, &claim.Locator)
			return claim, err
		})
		return err
	})
	return out, err
}
