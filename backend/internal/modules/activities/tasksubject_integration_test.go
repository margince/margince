// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func loggedTask(ctx context.Context, t *testing.T, e *sendEnv, subject string) crmcontracts.Activity {
	t.Helper()
	in, err := TaskInputFrom(crmcontracts.CreateTaskRequest{Subject: subject, Source: "manual"})
	if err != nil {
		t.Fatalf("TaskInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return activity
}

func TestATaskSubjectIsStoredTrimmed(t *testing.T) {
	e := setupSend(t)
	activity := loggedTask(e.as(principal.RowScopeAll), t, e, "  Call the buyer \n")
	if activity.Subject == nil || *activity.Subject != "Call the buyer" {
		t.Errorf("subject = %v, want the trimmed text", deref(activity.Subject))
	}
}

func TestABlankSubjectPatchOnATaskIsRefusedAndAPaddedOneIsTrimmed(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	task := loggedTask(ctx, t, e, "Call the buyer")
	id := ids.From[ids.ActivityKind](ids.UUID(task.Id))

	blank := "   "
	_, err := e.store(nil).UpdateActivity(ctx, id, UpdateActivityInput{Subject: &blank})
	var required *RequiredFieldError
	if !errors.As(err, &required) || required.Field != "subject" {
		t.Fatalf("err = %v, want a required-field refusal naming subject", err)
	}

	padded := "  Call the buyer back  "
	updated, err := e.store(nil).UpdateActivity(ctx, id, UpdateActivityInput{Subject: &padded})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if updated.Subject == nil || *updated.Subject != "Call the buyer back" {
		t.Errorf("subject = %v, want the trimmed text", deref(updated.Subject))
	}
}

func TestABlankSubjectPatchOnANoteKeepsItsBehaviour(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	note := loggedNote(ctx, t, e)

	blank := "   "
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(note.Id)),
		UpdateActivityInput{Subject: &blank}); err != nil {
		t.Fatalf("a note's subject stays editable to blank: %v", err)
	}
}
