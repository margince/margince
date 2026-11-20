// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const taskReopenedBy = "task_reopened_by"

// CloseTasksAnsweredByBookingTx completes every open task the booking's host
// holds on the lead or contact the booking was filed on, whoever created it. The
// booker is anonymous, so a task of any other seat is never theirs to close. It runs through the same
// completion and receipt evidence a sent mail uses. A task created at or after
// the booking, or one a user reopened after an automatic completion, stays open.
func (s *Store) CloseTasksAnsweredByBookingTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, host ids.UserID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	args := schedulingArgs{}
	query := `SELECT task.id FROM activity task JOIN activity booked ON booked.id=` + args.add(meeting) + `
 WHERE task.kind='task' AND NOT task.is_done AND task.archived_at IS NULL AND task.created_at < booked.created_at
 AND task.assignee_id=` + args.add(host) + `
 AND EXISTS (SELECT 1 FROM activity_link t JOIN activity_link m ON m.activity_id=booked.id
   AND (m.lead_id=t.lead_id OR m.contact_id=t.contact_id) WHERE t.activity_id=task.id)
 AND NOT ` + reopenedAfterAnswerSQL("task") + ` ORDER BY task.id FOR UPDATE OF task`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	tasks, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	return s.closeAnsweredTasksTx(ctx, tx, meeting, tasks)
}

// ReopenTasksAnsweredByMeeting reopens the tasks a booking closed that nobody
// has touched since, once the meeting is called off. The reopen carries its own
// evidence so a later mail may still answer the task.
func (s *Store) ReopenTasksAnsweredByMeeting(ctx context.Context, meeting ids.ActivityID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	reopened := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT DISTINCT task.id FROM activity task
 JOIN audit_log closed ON closed.entity_type='activity' AND closed.entity_id=task.id AND closed.evidence->>'`+taskAnsweredBy+`'=$2
 JOIN activity booked ON booked.id=$1 AND booked.kind='meeting' AND booked.meeting_status IN ('canceled','no_show')
 WHERE task.kind='task' AND task.is_done AND task.archived_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM audit_log later WHERE later.entity_type='activity' AND later.entity_id=task.id
   AND (later.occurred_at,later.id)>(closed.occurred_at,closed.id) AND later.after ? 'is_done')
 ORDER BY task.id`, meeting, meeting.String())
		if err != nil {
			return err
		}
		tasks, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return err
		}
		for _, id := range tasks {
			_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), UpdateActivityInput{
				IsDone: new(false), Trail: storekit.AuditTrail{Evidence: map[string]any{taskReopenedBy: meeting.String()}},
			})
			if err != nil {
				return err
			}
		}
		reopened = len(tasks)
		return nil
	})
	return reopened, err
}
