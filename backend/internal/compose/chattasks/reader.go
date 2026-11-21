// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chattasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Tasks is the activities store's side: which tasks a chat may change, and
// the write of one change.
type Tasks interface {
	ChatTasks(ctx context.Context, chat ids.UUID) ([]activities.ChatTask, time.Time, error)
	ApplyChatTaskChange(ctx context.Context, change activities.ChatTaskChange) (bool, error)
}

// Reader reads a filed chat against its record's open tasks and applies what
// it decided. A nil lane reads nothing, so with no model set up a chat is
// filed and no task changes.
type Reader struct {
	Lane  ai.Completer
	Tasks Tasks
}

// Settle reads one filed chat under the user who filed it and returns how
// many tasks it changed.
func (r Reader) Settle(ctx context.Context, chat ids.UUID, text string) (int, error) {
	if r.Lane == nil || strings.TrimSpace(text) == "" {
		return 0, nil
	}
	open, today, err := r.Tasks.ChatTasks(ctx, chat)
	if err != nil || len(open) == 0 {
		return 0, err
	}
	in := Input{Chat: text, Today: today}
	for _, task := range open {
		in.Tasks = append(in.Tasks, Task(task))
	}
	check := func(text string) error {
		_, err := Parse(text)
		return err
	}
	about := ai.WithSubject(ctx, ids.From[ids.ActivityKind](chat).Ref(), "filed chat")
	res, err := ai.Ask(about, r.Lane, Request(in, model.AboutActivities(append(taskIDs(open), chat)...)), check)
	if errors.Is(err, ai.ErrUnconfiguredModel) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("chat task reading: %w", err)
	}
	// Read again: a lane that cannot re-ask hands back the reply unchecked.
	answers, err := Parse(res.Text)
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, decision := range Decide(in, answers) {
		applied, err := r.Tasks.ApplyChatTaskChange(ctx, activities.ChatTaskChange{
			Chat: chat, Task: decision.Task, Done: decision.Done, DueOn: decision.DueOn, Quote: decision.Quote,
		})
		if err != nil {
			return changed, err
		}
		if applied {
			changed++
		}
	}
	return changed, nil
}

func taskIDs(tasks []activities.ChatTask) []ids.UUID {
	out := make([]ids.UUID, len(tasks))
	for i, task := range tasks {
		out[i] = task.ID
	}
	return out
}
