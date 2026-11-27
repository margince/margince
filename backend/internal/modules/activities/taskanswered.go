// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"maps"
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
	var stamp string
	var sender *ids.UUID
	args := schedulingArgs{}
	query := `SELECT sent.captured_by,` + answeringSenderSQL("sent") + ` FROM activity sent WHERE sent.id=` + args.add(email) + ` AND ` + answeringMailSQL("sent")
	if err := tx.QueryRow(ctx, query, args...).Scan(&stamp, &sender); errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	human := answeringHuman(stamp, sender)
	if human == "" {
		return 0, nil
	}
	tasks, err := tasksAnsweredByTx(ctx, tx, email, human, nil)
	if err != nil {
		return 0, err
	}
	return s.closeAnsweredTasksTx(ctx, tx, email, tasks, nil)
}

// answeringMailSQL is the mail side of the rule: our own attested, delivered,
// one-to-one mail.
func answeringMailSQL(sent string) string {
	return sent + `.kind='email' AND ` + sent + `.direction='outbound' AND ` + sent + `.archived_at IS NULL
 AND ` + sent + `.counterparty_outbound_attested AND NOT ` + sent + `.bulk_mail_attested
 AND NOT EXISTS (SELECT 1 FROM comms_outbound delivery WHERE delivery.activity_id=` + sent + `.id AND delivery.status<>'sent')`
}

// answeringSenderSQL names the seat that sent the mail. A guessed mailbox
// owner names nobody, and answeringHuman then falls back to the capture stamp.
func answeringSenderSQL(sent string) string {
	return `coalesce((SELECT d.user_id FROM comms_outbound d WHERE d.activity_id=` + sent + `.id AND d.status='sent' ORDER BY d.id LIMIT 1),(SELECT p.user_id FROM activity_participant p WHERE p.activity_id=` + sent + `.id AND p.role='from' AND p.user_id IS NOT NULL AND ` + auth.ParticipantStandsSQL("p", sent) + ` ORDER BY p.id LIMIT 1),` + sent + `.sent_by_colleague)`
}

func answeringHuman(stamp string, sender *ids.UUID) string {
	if sender != nil {
		return sender.String()
	}
	return actingHumanOf(stamp)
}

// answeredTaskSQL is the task side of the rule. The task is open, assigned to
// the sender, created before the mail's day and linked to a contact the mail
// was sent to. A task a user reopened after the rule closed it stays theirs.
func answeredTaskSQL(task, sent, human string) string {
	return task + `.kind='task' AND NOT ` + task + `.is_done AND ` + task + `.archived_at IS NULL
 AND ` + task + `.assignee_id::text=` + human + ` AND ` + occurredday.FromSQL(sent, task+".created_at + interval '1 microsecond'") + `
 AND EXISTS (SELECT 1 FROM activity_link target WHERE target.activity_id=` + task + `.id AND target.contact_id IS NOT NULL
   AND ` + recipientAddressedSQL(sent, "target.contact_id", "'to'") + `)
 AND NOT ` + reopenedAfterAnswerSQL(task)
}

// reopenedAfterAnswerSQL holds whichever mail closed the task: once a user
// reopened it after an automatic completion, no mail closes it again.
func reopenedAfterAnswerSQL(task string) string {
	return `EXISTS (SELECT 1 FROM audit_log answered JOIN audit_log reopened ON reopened.entity_type='activity' AND reopened.entity_id=answered.entity_id
 AND (reopened.occurred_at,reopened.id)>(answered.occurred_at,answered.id) AND reopened.after->>'is_done'='false'
 AND NOT coalesce(reopened.evidence ? '` + taskReopenedBy + `', false)
 WHERE answered.entity_type='activity' AND answered.entity_id=` + task + `.id AND answered.evidence ? '` + taskAnsweredBy + `')`
}

func tasksAnsweredByTx(ctx context.Context, tx pgx.Tx, email ids.ActivityID, human string, only *ids.UUID) ([]ids.UUID, error) {
	args := schedulingArgs{}
	query := `SELECT task.id FROM activity task JOIN activity sent ON sent.id=` + args.add(email) + `
 WHERE ` + answeredTaskSQL("task", "sent", args.add(human))
	if only != nil {
		query += ` AND task.id=` + args.add(*only)
	}
	rows, err := tx.Query(ctx, query+` ORDER BY task.id FOR UPDATE OF task`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// answerableTaskSQL is the task side the booking and chat doors share. It is
// an open task a user made, held by holder and made before the evidence row.
// It is linked to a record that onRecord admits over link alias t. A task a
// user reopened after an automatic answer stays theirs.
func answerableTaskSQL(args *schedulingArgs, evidence, holder, onRecord string) string {
	return `task.kind='task' AND NOT task.is_done AND task.archived_at IS NULL AND task.created_at < ` + evidence + `.created_at
 AND task.assignee_id=` + holder + `
 AND NOT (` + systemWordSQL("task", args.add(systemTaskSources)) + ` AND ` + systemCapturedSQL("task", args.add(systemCapturedBy), args.add(systemCapturedByPattern)) + `)
 AND EXISTS (SELECT 1 FROM activity_link t WHERE t.activity_id=task.id AND (` + onRecord + `))
 AND NOT ` + reopenedAfterAnswerSQL("task")
}

// closeAnsweredTasksTx completes the tasks with the evidence row as their
// answer. extra adds evidence keys of the door's own, such as a chat's line.
func (s *Store) closeAnsweredTasksTx(ctx context.Context, tx pgx.Tx, email ids.ActivityID, tasks []ids.UUID, extra map[string]any) (int, error) {
	at := s.now()
	for _, id := range tasks {
		evidence := map[string]any{taskAnsweredBy: email.String()}
		maps.Copy(evidence, extra)
		_, err := updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](id), UpdateActivityInput{
			IsDone: new(true), recordedAt: &at, recordZone: s.recordZone,
			Trail: storekit.AuditTrail{Evidence: evidence},
		})
		if err != nil {
			return 0, err
		}
	}
	return len(tasks), nil
}

// TaskEvidence is what kind of row answered a task.
type TaskEvidence string

// The rows that answer a task: our sent mail, a booked meeting, a filed chat.
const (
	EvidenceMail    TaskEvidence = "mail"
	EvidenceBooking TaskEvidence = "booking"
	EvidenceChat    TaskEvidence = "chat"
)

// TaskCompletion ties a task Margince changed to its evidence and the
// reversible audit entry. Moved is a new due day from a chat, not a completion.
type TaskCompletion struct {
	ID, AuditID, EvidenceID ids.UUID
	Subject, Contact        string
	At, WrittenAt           time.Time
	Version                 int64
	Writable, Moved         bool
	Evidence                TaskEvidence
	// Quote is the chat line the change rests on, empty when the reader may
	// not read that chat now. DueAt is the task's due day.
	Quote string
	DueAt *time.Time
	// quoteLine is the index of that line in the chat, -1 when none is stored.
	quoteLine int
}

// RecentTaskCompletions returns the caller's tasks a mail, booking or chat
// completed or moved and nobody changed since, with their reversible evidence.
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
		leadScope := "false"
		if auth.ReadGranted(ctx, "lead") {
			leadScope, err = auth.ScopeClauseFor(ctx, "lead", "ld", arg)
			if err != nil {
				return err
			}
			if leadScope == "" {
				leadScope = sqlTrue
			}
		}
		answered, moved := args.add(taskAnsweredBy), args.add(taskMovedBy)
		evidence := `coalesce(trail.evidence->>` + answered + `, trail.evidence->>` + moved + `)::uuid`
		query := `SELECT a.id, trail.id, ` + evidence + `,
    coalesce(a.subject,''), coalesce(c.full_name, lead_name.full_name, ''), trail.occurred_at, a.version, sent.occurred_at,
    CASE sent.kind WHEN 'meeting' THEN ` + args.add(EvidenceBooking) + ` WHEN 'message' THEN ` + args.add(EvidenceChat) + ` ELSE ` + args.add(EvidenceMail) + ` END,
    trail.evidence ? ` + moved + `, coalesce((trail.evidence->>` + args.add(taskAnswerLine) + `)::int, -1), ` + storekit.TaskDueSQL("a") + `
    FROM audit_log trail JOIN activity a ON a.id=trail.entity_id
 JOIN activity sent ON sent.id=` + evidence + `
    LEFT JOIN LATERAL (SELECT p.full_name FROM activity_link l JOIN contact p ON p.id=l.contact_id WHERE l.activity_id=a.id AND p.archived_at IS NULL AND (` + contactScope + `) ORDER BY p.id LIMIT 1) c ON true
    LEFT JOIN LATERAL (SELECT ld.full_name FROM activity_link l JOIN lead ld ON ld.id=l.lead_id WHERE l.activity_id=a.id AND ld.archived_at IS NULL AND (` + leadScope + `) ORDER BY ld.id LIMIT 1) lead_name ON true
    WHERE trail.entity_type='activity' AND (trail.evidence ? ` + answered + ` OR trail.evidence ? ` + moved + `)
      AND trail.occurred_at >= ` + args.add(since) + ` AND a.assignee_id=` + args.add(actor.UserID) + `
      AND a.kind='task' AND a.is_done <> (trail.evidence ? ` + moved + `) AND a.archived_at IS NULL AND (` + readable + `)
      AND NOT EXISTS (SELECT 1 FROM audit_log later WHERE later.entity_type='activity' AND later.entity_id=a.id AND (later.occurred_at,later.id)>(trail.occurred_at,trail.id)
        AND (later.after ? 'is_done' OR (trail.evidence ? ` + moved + ` AND later.after ? 'due_at')))
    ORDER BY trail.occurred_at DESC,trail.id DESC LIMIT ` + args.add(limit)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		candidates, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TaskCompletion, error) {
			var one TaskCompletion
			err := row.Scan(&one.ID, &one.AuditID, &one.EvidenceID, &one.Subject, &one.Contact, &one.At, &one.Version, &one.WrittenAt,
				&one.Evidence, &one.Moved, &one.quoteLine, &one.DueAt)
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
	return out, s.markWritableCompletions(ctx, out)
}

// markWritableCompletions says which receipts the caller may undo.
func (s *Store) markWritableCompletions(ctx context.Context, out []TaskCompletion) error {
	named := make([]ids.UUID, len(out))
	for i := range out {
		named[i] = out[i].ID
	}
	writable, err := s.WritableTasks(ctx, named)
	if err != nil {
		return err
	}
	for i := range out {
		out[i].Writable = writable[out[i].ID]
	}
	return nil
}

// chatQuote is the chat line a receipt rests on, read from the live chat only
// when the reader may read it. An archived, restricted or erased chat, or one
// the reader may not see, leaves the receipt without its quote.
func chatQuote(ctx context.Context, tx pgx.Tx, one TaskCompletion) (string, error) {
	if one.Evidence != EvidenceChat || one.quoteLine < 0 {
		return "", nil
	}
	chat, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](one.EvidenceID), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return "", nil
	}
	if err != nil || chat.Body == nil {
		return "", err
	}
	lines := ChatLines(*chat.Body)
	if one.quoteLine >= len(lines) {
		return "", nil
	}
	return lines[one.quoteLine], nil
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
		if one.DueAt != nil {
			due := one.DueAt.In(zone)
			one.DueAt = &due
		}
		if _, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](one.ID), storekit.LiveOnly); errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		} else if err != nil {
			return nil, err
		}
		if one.Quote, err = chatQuote(ctx, tx, one); err != nil {
			return nil, err
		}
		out = append(out, one)
	}
	return out, nil
}
