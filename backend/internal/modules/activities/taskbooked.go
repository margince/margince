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
	tasks, err := collectTaskIDs(ctx, tx, query, args)
	if err != nil {
		return 0, err
	}
	return s.closeAnsweredTasksTx(ctx, tx, meeting, tasks)
}

// answeredByCalledOffMeetingSQL admits a done task whose latest system answer
// or reopen is this called-off meeting's answer, and that no user wrote to since.
func answeredByCalledOffMeetingSQL(args *schedulingArgs, meeting ids.ActivityID) string {
	return `SELECT task.id FROM activity task WHERE task.kind='task' AND task.is_done AND task.archived_at IS NULL
 AND EXISTS (SELECT 1 FROM audit_log closed JOIN activity booked ON booked.id=` + args.add(meeting) + ` AND booked.kind='meeting' AND booked.meeting_status IN ('canceled','no_show')
   WHERE closed.entity_type='activity' AND closed.entity_id=task.id AND closed.evidence->>'` + taskAnsweredBy + `'=` + args.add(meeting.String()) + `
   AND NOT EXISTS (SELECT 1 FROM audit_log later WHERE later.entity_type='activity' AND later.entity_id=task.id
     AND (later.occurred_at,later.id)>(closed.occurred_at,closed.id)
     AND (later.actor_type<>'system' OR later.evidence ? '` + taskAnsweredBy + `' OR later.evidence ? '` + taskReopenedBy + `')))`
}

// ReopenTasksAnsweredByMeeting reopens the tasks a canceled booking closed.
// A task that a user or a later answer wrote to since stays as it is.
// The check runs again under the lock. A statement that waited on a row lock
// keeps its old snapshot and misses the audit row of the edit it waited for.
// The reopen carries its own evidence so a later mail may still answer the task.
func (s *Store) ReopenTasksAnsweredByMeeting(ctx context.Context, meeting ids.ActivityID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	reopened := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		lockArgs := schedulingArgs{}
		locked, err := collectTaskIDs(ctx, tx, answeredByCalledOffMeetingSQL(&lockArgs, meeting)+` ORDER BY task.id FOR UPDATE OF task`, lockArgs)
		if err != nil || len(locked) == 0 {
			return err
		}
		args := schedulingArgs{}
		query := answeredByCalledOffMeetingSQL(&args, meeting)
		tasks, err := collectTaskIDs(ctx, tx, query+` AND task.id=ANY(`+args.add(locked)+`) ORDER BY task.id`, args)
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

func collectTaskIDs(ctx context.Context, tx pgx.Tx, query string, args schedulingArgs) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}
