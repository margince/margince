// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// OpenTasksAssignedTo counts the open tasks from holds that the caller can see.
// A task whose linked records the caller cannot read is not theirs to count or
// move, as for every other activity read.
func (s *Store) OpenTasksAssignedTo(ctx context.Context, tx pgx.Tx, from ids.UserID) (int, error) {
	candidates, err := openTasksAssignedTo(ctx, tx, from)
	return len(candidates), err
}

// HandOverOpenTasksTx gives every open task from holds, that the caller may
// change, to the receiver, and answers how many moved. Each task moves through
// the ordinary update write and gets its audit row and event. A task the caller
// may not write stays where it is, and so do all of them for a caller without
// the update grant, so the records of the same handover still move.
//
// The receiver must hold work as for a single reassignment: a live human seat,
// not an agent.
func (s *Store) HandOverOpenTasksTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return 0, nil
		}
		return 0, err
	}
	candidates, err := openTasksAssignedTo(ctx, tx, from)
	if err != nil {
		return 0, err
	}
	at := s.now()
	moved := 0
	for _, id := range candidates {
		in := UpdateActivityInput{AssigneeID: &to, recordedAt: &at, recordZone: s.recordZone}
		_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), in)
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("activities: handing task %s to its new assignee: %w", id, err)
		}
		moved++
	}
	return moved, nil
}

// openTasksAssignedTo selects the live, undone tasks assigned to from that the
// caller may read.
func openTasksAssignedTo(ctx context.Context, tx pgx.Tx, from ids.UserID) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	args := []any{from}
	arg := func(v any) int { args = append(args, v); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if content == "" {
		content = sqlTrue
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id FROM activity a
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL
		   AND a.assignee_id = $1 AND (%s)
		 ORDER BY a.id`, content), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the open tasks to hand over: %w", err)
	}
	defer rows.Close()
	var found []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("activities: reading the open tasks to hand over: %w", err)
		}
		found = append(found, id)
	}
	return found, rows.Err()
}
