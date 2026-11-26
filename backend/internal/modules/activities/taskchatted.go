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
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The evidence keys a chat's change carries. A completion reuses
// taskAnsweredBy, so the reopen guard and the receipt read it as they read a
// mail's. A new date has its own key, because the task stays open. The line
// is an index into the chat, so the audit row holds none of its words.
const (
	taskMovedBy    = "task_moved_by"
	taskAnswerLine = "task_answer_line"
)

// ChatTaskReaderActor is the principal a chat's change to a task is written
// under, on behalf of the user who filed the chat.
const ChatTaskReaderActor = "system:chat-task-reader"

// MaxChatTasks bounds how many open tasks one chat is read against.
const MaxChatTasks = 10

// MinChatQuoteRunes is the shortest chat line a change may rest on. A line
// as short as "ok" or "yes" says nothing about which task it answers.
const MinChatQuoteRunes = 8

// ChatLines are the chat's lines, trimmed, in order. A change names one of
// them by its index, and the receipt reads it back by the same index.
func ChatLines(body string) []string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return lines
}

// ChatTask is one open task a filed chat may change.
type ChatTask struct {
	ID      ids.UUID
	Subject string
	DueAt   *time.Time
}

// ChatReading is what one filed chat is read with: the chat's stored text,
// the open tasks it may change, and today in the record zone.
type ChatReading struct {
	Body  string
	Tasks []ChatTask
	Today time.Time
}

// ChatTaskChange is what the reading decided for one task: done, or a new due
// day. Line is the index of the chat line the change rests on.
type ChatTaskChange struct {
	Chat, Task ids.UUID
	Done       bool
	DueOn      *time.Time
	Line       int
}

// chatTasksSQL selects the open tasks a filed chat answers for: the caller's
// own, on a record the chat is filed on. A chat is read once, when it is first
// filed, so it never changes a task a second time.
func chatTasksSQL(args *schedulingArgs, chat ids.UUID, holder ids.UUID) string {
	chatPos := args.add(chat)
	onRecord := `EXISTS (SELECT 1 FROM activity_link c WHERE c.activity_id=chat.id
   AND (t.contact_id=c.contact_id OR t.lead_id=c.lead_id OR t.deal_id=c.deal_id))`
	return `SELECT task.id FROM activity task JOIN activity chat ON chat.id=` + chatPos + `
 WHERE chat.kind='message' AND chat.source_system=` + args.add(ChatCaptureSourceSystem) + ` AND chat.archived_at IS NULL
 AND ` + answerableTaskSQL(args, "chat", args.add(holder), onRecord)
}

// ChatTasks returns the chat's stored text and the open tasks it may change,
// soonest due first, among those the caller may write.
func (s *Store) ChatTasks(ctx context.Context, chat ids.UUID) (ChatReading, error) {
	holder, err := chatTaskHolder(ctx)
	if err != nil {
		return ChatReading{}, err
	}
	var out ChatReading
	err = s.tx(ctx, func(tx pgx.Tx) error {
		zone, err := s.zoneOf(ctx, tx)
		if err != nil {
			return err
		}
		out.Today = s.now().In(zone)
		if out.Body, err = chatBody(ctx, tx, chat); err != nil || strings.TrimSpace(out.Body) == "" {
			return err
		}
		out.Tasks, err = openChatTasks(ctx, tx, chat, holder, zone)
		return err
	})
	return out, err
}

// chatBody is the filed chat's stored text, read as the caller may read it.
func chatBody(ctx context.Context, tx pgx.Tx, chat ids.UUID) (string, error) {
	filed, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](chat), storekit.LiveOnly)
	if err != nil || filed.Body == nil {
		return "", err
	}
	return *filed.Body, nil
}

func openChatTasks(ctx context.Context, tx pgx.Tx, chat, holder ids.UUID, zone *time.Location) ([]ChatTask, error) {
	args := schedulingArgs{}
	content, err := auth.ActivityContentClause(ctx, "task", func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return nil, err
	}
	due := storekit.TaskDueSQL("task")
	query := `SELECT task.id, coalesce(task.subject,''), ` + due + ` FROM activity task WHERE task.id IN (` +
		chatTasksSQL(&args, chat, holder) + `) AND (` + content + `) ORDER BY ` + due + ` NULLS LAST, task.id LIMIT ` + args.add(MaxChatTasks)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	return writableChatTasks(ctx, tx, found)
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
// again, under the caller, whether this chat may still change it. The line
// must still be one the stored chat holds. Otherwise nothing is written and
// the answer is false. The write runs as the chat task reader for the caller.
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
		body, err := chatBody(ctx, tx, change.Chat)
		if err != nil || !quotableLine(body, change.Line) {
			return err
		}
		applied, err = s.writeChatTaskChange(chatTaskReader(ctx, holder), tx, change)
		return err
	})
	return applied, err
}

// quotableLine reports whether the chat holds a line at index that is long
// enough for a change to rest on.
func quotableLine(body string, index int) bool {
	lines := ChatLines(body)
	return index >= 0 && index < len(lines) && utf8.RuneCountInString(lines[index]) >= MinChatQuoteRunes
}

func (s *Store) writeChatTaskChange(ctx context.Context, tx pgx.Tx, change ChatTaskChange) (bool, error) {
	line := map[string]any{taskAnswerLine: change.Line}
	if change.Done {
		closed, err := s.closeAnsweredTasksTx(ctx, tx, ids.From[ids.ActivityKind](change.Chat), []ids.UUID{change.Task}, line)
		return closed == 1, err
	}
	zone, err := s.zoneOf(ctx, tx)
	if err != nil {
		return false, err
	}
	at := s.now()
	day := change.DueOn.UTC()
	due := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone)
	line[taskMovedBy] = change.Chat.String()
	_, err = updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](change.Task), UpdateActivityInput{
		DueAt: &due, DueHasTime: new(false), recordedAt: &at, recordZone: s.recordZone,
		Trail: storekit.AuditTrail{Evidence: line},
	})
	return err == nil, err
}

// chatTaskReader is the principal the change is written under: the system,
// acting for the user who filed the chat, as the chat's own reader is.
func chatTaskReader(ctx context.Context, holder ids.UUID) context.Context {
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: ChatTaskReaderActor, UserID: holder, OnBehalfOf: holder,
	})
}
