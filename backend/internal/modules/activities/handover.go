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
	candidates, err := openTasksAssignedTo(ctx, tx, from, nil)
	return len(candidates), err
}

// HandOverOpenTasksTx gives every open task from holds, that the caller may
// change, to the receiver, and answers how many moved. Each task moves through
// the ordinary update write and gets its audit row and event. A task the caller
// may not write stays where it is. A caller without the update grant moves none,
// so the records of the same handover still move.
//
// The receiver must hold work as for a single reassignment: a live human seat,
// not an agent.
func (s *Store) HandOverOpenTasksTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, nilIfDenied(err)
	}
	candidates, err := openTasksAssignedTo(ctx, tx, from, nil)
	if err != nil {
		return 0, err
	}
	moved, _, err := s.handOverTasks(ctx, tx, candidates, to)
	return moved, err
}

// RecordLink names the contact or company a task is filed under.
type RecordLink struct {
	EntityType string
	ID         ids.UUID
}

// HandOverRecordTasksTx is HandOverOpenTasksTx for the open tasks from holds
// on one record. It also answers how many stayed, because their own write
// refused the move. A task from does not hold, or one already done, is not
// touched.
func (s *Store) HandOverRecordTasksTx(ctx context.Context, tx pgx.Tx, record RecordLink, from, to ids.UserID) (moved, kept int, err error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, 0, nilIfDenied(err)
	}
	candidates, err := openTasksAssignedTo(ctx, tx, from, &record)
	if err != nil {
		return 0, 0, err
	}
	return s.handOverTasks(ctx, tx, candidates, to)
}

// CountRecordTasksHeldByTx counts the open tasks holder holds on one record:
// the tasks HandOverRecordTasksTx would try to move. It is asked for a card
// only that holder reads, so it counts every such task rather than those the
// caller may open.
func (s *Store) CountRecordTasksHeldByTx(ctx context.Context, tx pgx.Tx, record RecordLink, holder ids.UserID) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return 0, err
	}
	args := []any{holder}
	arg := func(v any) int { args = append(args, v); return len(args) }
	filedUnder, err := filedUnderClause(&record, arg)
	if err != nil {
		return 0, err
	}
	var held int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM activity a
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND a.assignee_id = $1`+filedUnder,
		args...).Scan(&held); err != nil {
		return 0, fmt.Errorf("activities: counting a record's open tasks: %w", err)
	}
	return held, nil
}

// handOverTasks moves each task through the ordinary update write. A task its
// write refuses stays where it is and is counted as kept.
func (s *Store) handOverTasks(ctx context.Context, tx pgx.Tx, candidates []ids.UUID, to ids.UserID) (moved, kept int, err error) {
	at := s.now()
	for _, id := range candidates {
		in := UpdateActivityInput{AssigneeID: &to, recordedAt: &at, recordZone: s.recordZone}
		_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), in)
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			kept++
			continue
		}
		if err != nil {
			return 0, 0, fmt.Errorf("activities: handing task %s to its new assignee: %w", id, err)
		}
		moved++
	}
	return moved, kept, nil
}

// nilIfDenied reads a missing grant as nothing to hand over, and passes any
// other failure on.
func nilIfDenied(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
}

// openTasksAssignedTo selects the live, undone tasks assigned to from that the
// caller may read, filed under record when one is named.
func openTasksAssignedTo(ctx context.Context, tx pgx.Tx, from ids.UserID, record *RecordLink) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, nilIfDenied(err)
	}
	args := []any{from}
	arg := func(v any) int { args = append(args, v); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	filedUnder, err := filedUnderClause(record, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id FROM activity a
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL
		   AND a.assignee_id = $1 AND (%s)%s
		 ORDER BY a.id`, content, filedUnder), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the open tasks to hand over: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading the open tasks to hand over: %w", err)
	}
	return found, nil
}

// recordLinkColumns are the activity_link columns that name a contact or a
// company, keyed by the link's entity type.
var recordLinkColumns = map[string]string{"contact": "contact_id", "company": "company_id"}

// filedUnderClause narrows the tasks to those linked to record; empty for none.
func filedUnderClause(record *RecordLink, arg func(any) int) (string, error) {
	if record == nil {
		return "", nil
	}
	column, ok := recordLinkColumns[record.EntityType]
	if !ok {
		return "", fmt.Errorf("activities: tasks are not filed under a %q", record.EntityType)
	}
	return fmt.Sprintf(` AND EXISTS (SELECT 1 FROM activity_link l
		 WHERE l.activity_id = a.id AND l.entity_type = $%d AND l.%s = $%d)`,
		arg(record.EntityType), column, arg(record.ID)), nil
}
