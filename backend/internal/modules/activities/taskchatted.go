// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A filed chat screenshot as evidence for the open tasks on its record. The
// model's reading lives in compose. This file says which tasks it may read
// and writes what it decided. The chat and the quoted line are the evidence,
// so the receipt and its Undo come from the audit row.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The evidence keys a chat's change carries. A completion reuses
// taskAnsweredBy, so the reopen guard and the receipt read it as they read a
// mail's. A new date has its own key, because the task stays open.
const (
	taskMovedBy     = "task_moved_by"
	taskAnswerQuote = "task_answer_quote"
)

// ChatTaskReaderActor is the principal a chat's change to a task is written
// under, on behalf of the user who filed the chat.
const ChatTaskReaderActor = "system:chat-task-reader"

// MaxChatTasks bounds how many open tasks one chat is read against.
const MaxChatTasks = 10

// ChatTask is one open task a filed chat may change.
type ChatTask struct {
	ID      ids.UUID
	Subject string
	DueAt   *time.Time
}

// ChatTaskChange is what the reading decided for one task: done, or a new due
// day. Quote is the chat's own sentence the change rests on.
type ChatTaskChange struct {
	Chat, Task ids.UUID
	Done       bool
	DueOn      *time.Time
	Quote      string
}

// chatTasksSQL selects the open tasks a filed chat answers for. They are the
// caller's own, made before the chat, on a record the chat is filed on.
// Margince's own tasks keep their own resolution. A task a user reopened
// after an automatic answer, or one this chat already changed, stays as it is.
func chatTasksSQL(args *schedulingArgs, chat ids.UUID, holder ids.UUID) string {
	chatPos, chatText := args.add(chat), args.add(chat.String())
	return `SELECT task.id FROM activity task JOIN activity chat ON chat.id=` + chatPos + `
 WHERE chat.kind='message' AND chat.source_system=` + args.add(ChatCaptureSourceSystem) + ` AND chat.archived_at IS NULL
 AND task.kind='task' AND NOT task.is_done AND task.archived_at IS NULL AND task.created_at < chat.created_at
 AND task.assignee_id=` + args.add(holder) + `
 AND NOT (task.source=` + args.add(systemSource) + ` AND ` + systemCapturedSQL("task", args.add(systemCapturedBy), args.add(systemCapturedByPattern)) + `)
 AND EXISTS (SELECT 1 FROM activity_link t JOIN activity_link c ON c.activity_id=chat.id
   WHERE t.activity_id=task.id AND (t.contact_id=c.contact_id OR t.lead_id=c.lead_id OR t.deal_id=c.deal_id))
 AND NOT ` + reopenedAfterAnswerSQL("task") + `
 AND NOT EXISTS (SELECT 1 FROM audit_log seen WHERE seen.entity_type='activity' AND seen.entity_id=task.id
   AND (seen.evidence->>'` + taskAnsweredBy + `'=` + chatText + ` OR seen.evidence->>'` + taskMovedBy + `'=` + chatText + `))`
}

// ChatTasks returns the open tasks a filed chat may change, soonest due
// first, among those the caller may write, and today in the record zone.
func (s *Store) ChatTasks(ctx context.Context, chat ids.UUID) ([]ChatTask, time.Time, error) {
	holder, err := chatTaskHolder(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	var out []ChatTask
	var today time.Time
	err = s.tx(ctx, func(tx pgx.Tx) error {
		zone, err := s.zoneOf(ctx, tx)
		if err != nil {
			return err
		}
		today = s.now().In(zone)
		args := schedulingArgs{}
		content, err := auth.ActivityContentClause(ctx, "task", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		due := storekit.TaskDueSQL("task")
		query := `SELECT task.id, coalesce(task.subject,''), ` + due + ` FROM activity task WHERE task.id IN (` +
			chatTasksSQL(&args, chat, holder) + `) AND (` + content + `) ORDER BY ` + due + ` NULLS LAST, task.id LIMIT ` + args.add(MaxChatTasks)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ChatTask, error) {
			var task ChatTask
			err := row.Scan(&task.ID, &task.Subject, &task.DueAt)
			if task.DueAt != nil {
				due := task.DueAt.In(zone)
				task.DueAt = &due
			}
			return task, err
		})
		if err != nil {
			return err
		}
		out, err = writableChatTasks(ctx, tx, found)
		return err
	})
	return out, today, err
}

// zoneOf is the record zone, or UTC on a store wired without one.
func (s *Store) zoneOf(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	if s.recordZone == nil {
		return time.UTC, nil
	}
	return s.recordZone(ctx, tx)
}

func writableChatTasks(ctx context.Context, tx pgx.Tx, found []ChatTask) ([]ChatTask, error) {
	named := make([]ids.UUID, len(found))
	for i, task := range found {
		named[i] = task.ID
	}
	writable, err := auth.ActivityWritableSubset(ctx, tx, named)
	if err != nil {
		return nil, err
	}
	out := []ChatTask{}
	for _, task := range found {
		if writable[task.ID] {
			out = append(out, task)
		}
	}
	return out, nil
}

// chatTaskHolder is the human the chat was filed by: only their tasks change.
func chatTaskHolder(ctx context.Context) (ids.UUID, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return ids.UUID{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

// ApplyChatTaskChange writes one decided change. The task is locked and asked
// again, under the caller, whether this chat may still change it. A task that
// moved on since the reading answers false, and nothing is written. The write
// runs as the chat task reader on the caller's behalf.
func (s *Store) ApplyChatTaskChange(ctx context.Context, change ChatTaskChange) (bool, error) {
	holder, err := chatTaskHolder(ctx)
	if err != nil {
		return false, err
	}
	if change.Done == (change.DueOn != nil) {
		return false, errors.New("activities: a chat's change to a task is either done or a new day")
	}
	applied := false
	err = s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		query := chatTasksSQL(&args, change.Chat, holder) + ` AND task.id=` + args.add(change.Task) + ` FOR UPDATE OF task`
		still, err := collectTaskIDs(ctx, tx, query, args)
		if err != nil || len(still) == 0 {
			return err
		}
		if writable, err := writableChatTasks(ctx, tx, []ChatTask{{ID: change.Task}}); err != nil || len(writable) == 0 {
			return err
		}
		in, err := s.chatTaskPatch(ctx, tx, change)
		if err != nil {
			return err
		}
		if _, err := updateActivityInTx(chatTaskReader(ctx, holder), tx, ids.From[ids.ActivityKind](change.Task), in); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied, err
}

func (s *Store) chatTaskPatch(ctx context.Context, tx pgx.Tx, change ChatTaskChange) (UpdateActivityInput, error) {
	at := s.now()
	in := UpdateActivityInput{recordedAt: &at, recordZone: s.recordZone}
	if change.Done {
		in.IsDone = new(true)
		in.Trail = storekit.AuditTrail{Evidence: map[string]any{taskAnsweredBy: change.Chat.String(), taskAnswerQuote: change.Quote}}
		return in, nil
	}
	zone, err := s.zoneOf(ctx, tx)
	if err != nil {
		return in, err
	}
	day := change.DueOn.UTC()
	due := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	in.DueAt, in.DueHasTime = &due, new(false)
	in.Trail = storekit.AuditTrail{Evidence: map[string]any{taskMovedBy: change.Chat.String(), taskAnswerQuote: change.Quote}}
	return in, nil
}

// chatTaskReader is the principal the change is written under: the system,
// acting for the user who filed the chat, as the chat's own reader is.
func chatTaskReader(ctx context.Context, holder ids.UUID) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: ChatTaskReaderActor, UserID: holder, OnBehalfOf: holder,
	})
}
