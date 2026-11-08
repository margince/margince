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
// may not write stays where it is. A caller without the update grant moves none,
// so the records of the same handover still move.
//
// The receiver must hold work as for a single reassignment: a live human seat,
// not an agent.
func (s *Store) HandOverOpenTasksTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, nilIfDenied(err)
	}
	candidates, err := openTasksAssignedTo(ctx, tx, from)
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

// RecordTasks is what handing on a record's open tasks did, or would do.
type RecordTasks struct {
	Moved, Kept int
}

// HandOverRecordTasksTx is HandOverOpenTasksTx for the open tasks from holds
// on one record, by the rule recordTasksTx states. A task the rule keeps, or
// whose own write refuses the move, stays with from and is counted in Kept.
func (s *Store) HandOverRecordTasksTx(ctx context.Context, tx pgx.Tx, record RecordLink, from, to ids.UserID) (RecordTasks, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return RecordTasks{}, nilIfDenied(err)
	}
	held, err := recordTasksTx(ctx, tx, record, from)
	if err != nil {
		return RecordTasks{}, err
	}
	var moving []ids.UUID
	out := RecordTasks{}
	for _, task := range held {
		if task.Moves {
			moving = append(moving, task.ID)
		} else {
			out.Kept++
		}
	}
	moved, refused, err := s.handOverTasks(ctx, tx, moving, to)
	out.Moved, out.Kept = moved, out.Kept+refused
	return out, err
}

// CountRecordTasksHeldByTx answers what HandOverRecordTasksTx would do with
// the open tasks holder holds on one record.
func (s *Store) CountRecordTasksHeldByTx(ctx context.Context, tx pgx.Tx, record RecordLink, holder ids.UserID) (RecordTasks, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return RecordTasks{}, err
	}
	held, err := recordTasksTx(ctx, tx, record, holder)
	if err != nil {
		return RecordTasks{}, err
	}
	var out RecordTasks
	for _, task := range held {
		if task.Moves {
			out.Moved++
		} else {
			out.Kept++
		}
	}
	return out, nil
}

// recordTask is one open task the holder holds on the record.
type recordTask struct {
	ID    ids.UUID
	Moves bool
}

// recordTasksTx reads the open tasks holder holds on one record, and whether
// each goes with it. Only a task whose every link names this record goes. A
// task also filed under another record is that record's business too.
// The count and the move both read through it. It reads every such task, not
// only those the caller may open. The count goes onto a card only the holder
// reads, and each move passes the task's own write gate.
func recordTasksTx(ctx context.Context, tx pgx.Tx, record RecordLink, holder ids.UserID) ([]recordTask, error) {
	column, ok := recordLinkColumns[record.EntityType]
	if !ok {
		return nil, fmt.Errorf("activities: tasks are not filed under a %q", record.EntityType)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id, NOT EXISTS (SELECT 1 FROM activity_link o WHERE o.activity_id = a.id		         AND (o.entity_type <> $2 OR o.%[1]s IS DISTINCT FROM $3))
		  FROM activity a
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND a.assignee_id = $1
		   AND EXISTS (SELECT 1 FROM activity_link l
		                WHERE l.activity_id = a.id AND l.entity_type = $2 AND l.%[1]s = $3)
		 ORDER BY a.id`, column), holder, record.EntityType, record.ID)
	if err != nil {
		return nil, fmt.Errorf("activities: reading a record's open tasks: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[recordTask])
	if err != nil {
		return nil, fmt.Errorf("activities: reading a record's open tasks: %w", err)
	}
	return found, nil
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
// caller may read.
func openTasksAssignedTo(ctx context.Context, tx pgx.Tx, from ids.UserID) ([]ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, nilIfDenied(err)
	}
	args := []any{from}
	arg := func(v any) int { args = append(args, v); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT a.id FROM activity a
		 WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL
		   AND a.assignee_id = $1 AND (%s)
		 ORDER BY a.id`, content), args...)
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
var recordLinkColumns = map[string]string{"contact": "contact_id", linkEntityCompany: "company_id"}
