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

// writableBookedRecords are the leads and contacts the booking was filed on
// that the host may still write. A task on any other record is not the host's
// to settle for an anonymous booker.
func (s *Store) writableBookedRecords(ctx context.Context, tx pgx.Tx, links []ActivityLinkInput, host ids.UserID) (leads, contacts []ids.UUID, err error) {
	for _, link := range links {
		if link.EntityType != linkEntityLead && link.EntityType != linkEntityContact {
			continue
		}
		ok, err := s.calendar.HostMayWrite(ctx, tx, host, link.EntityType, link.EntityID)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case !ok:
		case link.EntityType == linkEntityLead:
			leads = append(leads, link.EntityID)
		default:
			contacts = append(contacts, link.EntityID)
		}
	}
	return leads, contacts, nil
}

// CloseTasksAnsweredByBookingTx completes the open tasks a user made for the
// booking's host on the lead or contact the booking was filed on. It uses the
// completion and receipt evidence a sent mail uses.
// The booker is anonymous, so only a record the host may write counts.
// A task of another seat, or one Margince made, keeps its own resolution.
// A task made after the booking, or reopened by a user, stays open.
func (s *Store) CloseTasksAnsweredByBookingTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, host ids.UserID, links []ActivityLinkInput) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	leads, contacts, err := s.writableBookedRecords(ctx, tx, links, host)
	if err != nil || len(leads)+len(contacts) == 0 {
		return 0, err
	}
	args := schedulingArgs{}
	booked := args.add(meeting)
	query := `SELECT task.id FROM activity task JOIN activity booked ON booked.id=` + booked + `
 WHERE task.kind='task' AND NOT task.is_done AND task.archived_at IS NULL AND task.created_at < booked.created_at
 AND task.assignee_id=` + args.add(host) + `
 AND NOT (task.source=` + args.add(systemSource) + ` AND ` + systemCapturedSQL("task", args.add(systemCapturedBy), args.add(systemCapturedByPattern)) + `)
 AND EXISTS (SELECT 1 FROM activity_link t WHERE t.activity_id=task.id AND (t.lead_id=ANY(` + args.add(leads) + `) OR t.contact_id=ANY(` + args.add(contacts) + `)))
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

// ReopenTasksAnsweredByMeeting reopens the tasks a booking closed once the
// meeting is canceled, unless a user wrote to the task since. The rows are
// locked before the decision, so a concurrent edit wins. The reopen carries its
// own evidence so a later mail may still answer the task.
func (s *Store) ReopenTasksAnsweredByMeeting(ctx context.Context, meeting ids.ActivityID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	reopened := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT task.id FROM activity task
 WHERE task.kind='task' AND task.is_done AND task.archived_at IS NULL
 AND EXISTS (SELECT 1 FROM audit_log closed JOIN activity booked ON booked.id=$1 AND booked.kind='meeting' AND booked.meeting_status IN ('canceled','no_show')
   WHERE closed.entity_type='activity' AND closed.entity_id=task.id AND closed.evidence->>'`+taskAnsweredBy+`'=$2
   AND NOT EXISTS (SELECT 1 FROM audit_log later WHERE later.entity_type='activity' AND later.entity_id=task.id
     AND (later.occurred_at,later.id)>(closed.occurred_at,closed.id) AND later.actor_type<>'system'))
 ORDER BY task.id FOR UPDATE OF task`, meeting, meeting.String())
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
