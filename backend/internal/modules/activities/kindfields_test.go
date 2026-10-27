// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An import and the create wire refuse the same fields of the same kind with
// the same error, because both ask CheckKindFields.
func TestTheImportAndTheWireRefuseTheSameKindFields(t *testing.T) {
	seconds := func(n int) *int { return &n }
	held := crmcontracts.CreateActivityRequestMeetingStatus("held")
	transport := "whatsapp"
	cases := map[string]struct {
		kind     string
		duration *int
		status   *crmcontracts.CreateActivityRequestMeetingStatus
		provider *string
	}{
		"no kind":                     {kind: ""},
		"a kind this timeline lacks":  {kind: "postcard"},
		"a duration on a note":        {kind: "note", duration: seconds(600)},
		"a status on a call":          {kind: "call", status: &held},
		"a message without a channel": {kind: "message"},
		"a transport on a note":       {kind: "note", provider: &transport},
		"a clean meeting":             {kind: "meeting", duration: seconds(1800), status: &held},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, wire := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
				Kind: crmcontracts.CreateActivityRequestKind(c.kind), Source: "test",
				DurationSeconds: c.duration, MeetingStatus: c.status, ChannelProvider: c.provider,
			})
			status := meetingStatusOf(c.status)
			imported := CheckKindFields(c.kind, c.duration, status, providerOf(c.provider))
			if fmt.Sprintf("%T", wire) != fmt.Sprintf("%T", imported) {
				t.Fatalf("wire refused with %T (%v), import with %T (%v)", wire, wire, imported, imported)
			}
		})
	}
}

// The wire keeps its error order. A blank source is reported before a kind
// field, and a status on a call before a transport.
func TestTheWireReportsASourceBeforeAKindField(t *testing.T) {
	seconds := 600
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKind("note"), Source: " ", DurationSeconds: &seconds,
	})
	if _, isKindField := errors.AsType[*KindFieldError](err); err == nil || isKindField {
		t.Fatalf("a blank source was answered with %v, want the source refusal", err)
	}
	held := crmcontracts.CreateActivityRequestMeetingStatus("held")
	transport := "whatsapp"
	_, err = LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: crmcontracts.CreateActivityRequestKind("note"), Source: "test",
		MeetingStatus: &held, ChannelProvider: &transport,
	})
	if _, ok := errors.AsType[*MeetingStatusKindError](err); !ok {
		t.Fatalf("a status on a note was answered with %v", err)
	}
}

// A task-only field on another kind is refused naming that field, on create
// and on update. A task may carry all of them.
func TestATaskOnlyFieldOnANoteNamesTheField(t *testing.T) {
	instant := clocktest.Now(t)
	due, done, notDone := instant, true, false
	assignee := ids.From[ids.UserKind](ids.NewV7())
	cases := map[string]struct {
		fields taskFields
		field  string
	}{
		"a due date":      {taskFields{dueAt: &due}, "due_at"},
		"a reminder":      {taskFields{remindAt: &due}, "remind_at"},
		"an assignee":     {taskFields{assignee: &assignee}, "assignee_id"},
		"a finished flag": {taskFields{isDone: &done}, "is_done"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			refusal, ok := errors.AsType[*KindFieldError](checkTaskFieldsKind("note", c.fields))
			if !ok || refusal.Field != c.field {
				t.Fatalf("a note was answered with %v, want a refusal naming %s", refusal, c.field)
			}
			if err := checkTaskFieldsKind("task", c.fields); err != nil {
				t.Errorf("a task was refused: %v", err)
			}
		})
	}
	if err := checkTaskFieldsKind("note", taskFields{isDone: &notDone}); err != nil {
		t.Errorf("is_done false is what a note already holds, got %v", err)
	}
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "note", Source: "test", DueAt: &due})
	if refusal, ok := errors.AsType[*KindFieldError](err); !ok || refusal.Field != "due_at" {
		t.Errorf("create of a note with a due date answered %v, want a refusal naming due_at", err)
	}
}
