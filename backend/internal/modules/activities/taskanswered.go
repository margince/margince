// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const taskAnsweredBy = "task_answered_by"

func (s *Store) completeContactTasksAnsweredBy(ctx context.Context, email ids.ActivityID) (int, error) {
	completed := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		completed, err = s.CompleteContactTasksAnsweredByTx(ctx, tx, email)
		return err
	})
	return completed, err
}

// CompleteContactTasksAnsweredByTx shares capture and durable-send completion on the caller's transaction.
func (s *Store) CompleteContactTasksAnsweredByTx(ctx context.Context, tx pgx.Tx, email ids.ActivityID) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	completed := 0
	var stamp string
	var sender *ids.UUID
	args := schedulingArgs{}
	query := `SELECT captured_by,coalesce((SELECT d.user_id FROM comms_outbound d WHERE d.activity_id=sent.id AND d.status='sent' ORDER BY d.id LIMIT 1),(SELECT p.user_id FROM activity_participant p WHERE p.activity_id=sent.id AND p.role='from' AND p.user_id IS NOT NULL ORDER BY p.id LIMIT 1),sent.sent_by_colleague) FROM activity sent WHERE id=` + args.add(email) + ` AND kind='email' AND direction='outbound'
    AND archived_at IS NULL AND counterparty_outbound_attested AND NOT bulk_mail_attested
 AND NOT EXISTS (SELECT 1 FROM comms_outbound delivery WHERE delivery.activity_id=sent.id AND delivery.status<>'sent')`
	if err := tx.QueryRow(ctx, query, args...).Scan(&stamp, &sender); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	human := actingHumanOf(stamp)
	if sender != nil {
		human = sender.String()
	}
	if human == "" {
		return 0, nil
	}
	args = schedulingArgs{}
	query = `SELECT task.id FROM activity task JOIN activity sent ON sent.id=` + args.add(email) + `
    WHERE task.kind='task' AND NOT task.is_done AND task.archived_at IS NULL
    AND task.assignee_id::text=` + args.add(human) + ` AND ` + occurredday.FromSQL("sent", "task.created_at + interval '1 microsecond'") + `
    AND EXISTS (SELECT 1 FROM activity_link target WHERE target.activity_id=task.id AND target.contact_id IS NOT NULL
      AND ` + promiseRecipientAddressedSQL("sent", "target.contact_id") + `)
    AND NOT EXISTS (SELECT 1 FROM audit_log prior WHERE prior.entity_type='activity' AND prior.entity_id=task.id
      AND prior.evidence->>` + args.add(taskAnsweredBy) + ` IN (
 WITH RECURSIVE answered_mail(id) AS (
 SELECT sent.id UNION SELECT folded.entity_id FROM audit_log folded JOIN answered_mail kept ON folded.evidence->>'` + meetingCopySurvivorKey + `'=kept.id::text
 WHERE folded.entity_type='activity' AND folded.action='archive' AND folded.evidence->>'` + evidenceReasonKey + `'='` + mailCopyReason + `') SELECT id::text FROM answered_mail))
    ORDER BY task.id FOR UPDATE OF task`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	tasks, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, err
	}
	at := s.now()
	for _, id := range tasks {
		_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), UpdateActivityInput{
			IsDone: new(true), recordedAt: &at, recordZone: s.recordZone,
			Trail: storekit.AuditTrail{Evidence: map[string]any{taskAnsweredBy: email.String()}},
		})
		if err != nil {
			return 0, err
		}
		completed++
	}
	return completed, nil
}

// TaskCompletion ties a completed task to the sent mail and reversible audit entry.
type TaskCompletion struct {
	ID, AuditID, EmailID ids.UUID
	Subject, Contact     string
	At, WrittenAt        time.Time
	Version              int64
	Writable             bool
}

// RecentTaskCompletions returns only the caller's still-completed tasks and their reversible evidence.
func (s *Store) RecentTaskCompletions(ctx context.Context, since time.Time, limit int) ([]TaskCompletion, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	out := []TaskCompletion{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		arg := func(value any) int { args = append(args, value); return len(args) }
		readable, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		contactScope := "false"
		if auth.ReadGranted(ctx, "contact") {
			contactScope, err = auth.ScopeClauseFor(ctx, "contact", "p", arg)
			if err != nil {
				return err
			}
			if contactScope == "" {
				contactScope = sqlTrue
			}
		}
		query := `SELECT a.id, trail.id, (trail.evidence->>` + args.add(taskAnsweredBy) + `)::uuid,
    coalesce(a.subject,''), coalesce(c.full_name,''), trail.occurred_at, a.version, sent.occurred_at
    FROM audit_log trail JOIN activity a ON a.id=trail.entity_id
 JOIN activity sent ON sent.id=(trail.evidence->>'task_answered_by')::uuid
    LEFT JOIN LATERAL (SELECT p.full_name FROM activity_link l JOIN contact p ON p.id=l.contact_id WHERE l.activity_id=a.id AND p.archived_at IS NULL AND (` + contactScope + `) ORDER BY p.id LIMIT 1) c ON true
    WHERE trail.entity_type='activity' AND trail.evidence ? ` + args.add(taskAnsweredBy) + `
      AND trail.occurred_at >= ` + args.add(since) + ` AND a.assignee_id=` + args.add(actor.UserID) + `
      AND a.kind='task' AND a.is_done AND a.archived_at IS NULL AND (` + readable + `)
      AND NOT EXISTS (SELECT 1 FROM audit_log later WHERE later.entity_type='activity' AND later.entity_id=a.id AND (later.occurred_at,later.id)>(trail.occurred_at,trail.id) AND later.after ? 'is_done')
    ORDER BY trail.occurred_at DESC,trail.id DESC LIMIT ` + args.add(limit)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskCompletion, error) {
			var one TaskCompletion
			err := row.Scan(&one.ID, &one.AuditID, &one.EmailID, &one.Subject, &one.Contact, &one.At, &one.Version, &one.WrittenAt)
			return one, err
		})
		if err != nil {
			return err
		}
		out, err = s.readableTaskCompletions(ctx, tx, candidates)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading task completion receipts: %w", err)
	}
	named := make([]ids.UUID, len(out))
	for i := range out {
		named[i] = out[i].ID
	}
	writable, err := s.WritableTasks(ctx, named)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Writable = writable[out[i].ID]
	}
	return out, nil
}

func (s *Store) readableTaskCompletions(ctx context.Context, tx pgx.Tx, candidates []TaskCompletion) ([]TaskCompletion, error) {
	out := []TaskCompletion{}
	var err error
	zone := time.UTC
	if s.recordZone != nil {
		zone, err = s.recordZone(ctx, tx)
		if err != nil {
			return nil, err
		}
	}
	for _, one := range candidates {
		one.WrittenAt = one.WrittenAt.In(zone)
		if _, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](one.ID), storekit.LiveOnly); errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		} else if err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	return out, nil
}
