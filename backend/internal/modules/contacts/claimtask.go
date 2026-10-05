// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ClaimOnTask is one open claim a task stands for, and the contact it is filed
// on, which is the record settling it writes.
type ClaimOnTask struct {
	ID      ids.UUID
	Contact ids.UUID
}

// OpenClaimsOnTask lists the open claims a task stands for. A claim and the
// task an extracted commitment became settle together, and this is the claim
// half of that question.
func (s *Store) OpenClaimsOnTask(ctx context.Context, taskID ids.UUID) ([]ClaimOnTask, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	taskPos := arg(taskID)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	var out []ClaimOnTask
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, fmt.Sprintf(`
			SELECT c.id, c.contact_id FROM conversation_claim c
			  JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
			 WHERE c.task_activity_id = $%d AND c.status = 'open' AND c.archived_at IS NULL
			   AND (%s)
			 ORDER BY c.id`, taskPos, scope), args...)
		if err != nil {
			return fmt.Errorf("read the claims on a task: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (ClaimOnTask, error) {
			var c ClaimOnTask
			err := row.Scan(&c.ID, &c.Contact)
			return c, err
		})
		return err
	})
	return out, err
}

// ClaimTask is the task a claim became, and the claim's status, or no task
// when the claim never became one or is gone.
func (s *Store) ClaimTask(ctx context.Context, claimID ids.UUID) (*ids.UUID, string, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, "", err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	claimPos := arg(claimID)
	scope, err := auth.ScopeClauseFor(ctx, "contact", "pr", arg)
	if err != nil {
		return nil, "", err
	}
	if scope == "" {
		scope = sqlAlwaysVisible
	}
	var task *ids.UUID
	var status string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT c.task_activity_id, c.status FROM conversation_claim c
			  JOIN contact pr ON pr.id = c.contact_id AND pr.archived_at IS NULL
			 WHERE c.id = $%d AND c.archived_at IS NULL AND (%s)`, claimPos, scope), args...).Scan(&task, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, "", fmt.Errorf("read the task a claim became: %w", err)
	}
	return task, status, nil
}
