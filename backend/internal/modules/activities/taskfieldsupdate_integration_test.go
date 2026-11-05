// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The activity_task_fields CHECK refuses a task-only field on another kind with
// an answer that names no field. The update path refuses it first, naming the
// field, so a caller knows which value to drop.

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnUpdateOfANoteNamesTheTaskFieldItCannotCarry(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	note := ids.From[ids.ActivityKind](ids.UUID(loggedNote(ctx, t, e).Id))
	due, done := clocktest.Now(t).Add(time.Hour), true
	assignee := ids.From[ids.UserKind](ids.NewV7())
	cases := map[string]struct {
		in    UpdateActivityInput
		field string
	}{
		"a due date":      {UpdateActivityInput{DueAt: &due}, "due_at"},
		"a reminder":      {UpdateActivityInput{RemindAt: &due}, "remind_at"},
		"an assignee":     {UpdateActivityInput{AssigneeID: &assignee}, "assignee_id"},
		"a finished flag": {UpdateActivityInput{IsDone: &done}, "is_done"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := e.store(nil).UpdateActivity(ctx, note, c.in)
			refusal, ok := errors.AsType[*KindFieldError](err)
			if !ok || refusal.Field != c.field {
				t.Fatalf("UpdateActivity answered %v, want a refusal naming %s", err, c.field)
			}
		})
	}
}
