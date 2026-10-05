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

// OpenClaimsOnTask lists the open claims a task stands for. A claim and the
// task an extracted commitment became settle together, and this is the claim
// half of that question.
func (s *Store) OpenClaimsOnTask(ctx context.Context, taskID ids.UUID) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []ids.UUID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM conversation_claim
			 WHERE task_activity_id = $1 AND status = 'open' AND archived_at IS NULL
			 ORDER BY id`, taskID)
		if err != nil {
			return fmt.Errorf("read the claims on a task: %w", err)
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
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
	var task *ids.UUID
	var status string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT task_activity_id, status FROM conversation_claim
			 WHERE id = $1 AND archived_at IS NULL`, claimID).Scan(&task, &status)
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
