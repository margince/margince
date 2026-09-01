// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who owns a task, written by the real writer and read back off the row.
//
// The unit test beside taskAssignee proves the decision; this proves the
// decision reaches the column. Without it the helper could be disconnected from
// the insert and every unit test would stay green while the queue quietly went
// back to showing one rep's work to everybody.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// storedAssignee reads the assignee off the row itself, not off the response,
// so the test cannot pass on a value the writer merely echoed back.
func storedAssignee(t *testing.T, id string) *ids.UUID {
	t.Helper()
	var assignee *ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT assignee_id FROM activity WHERE id = $1`, id).Scan(&assignee); err != nil {
		t.Fatalf("reading the task's assignee: %v", err)
	}
	return assignee
}

// A rep writing themselves a task owns it, without naming themselves. This is
// the case that lets "mine" be exact: the queue stopped folding in every
// ownerless row, so the row a rep wrote has to arrive owned.
func TestASelfWrittenTaskIsStoredAgainstItsAuthor(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	person := e.SeedPerson(t, "A Contact", &e.Rep1)

	task, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "task", Subject: strPtr("Call them back"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
	})
	if err != nil {
		t.Fatalf("logging the task: %v", err)
	}

	stored := storedAssignee(t, task.Id.String())
	if stored == nil {
		t.Fatal("a task the rep wrote for themselves was stored belonging to nobody")
	}
	if *stored != e.Rep1 {
		t.Fatalf("the task was stored against %v, wanted its author %v", *stored, e.Rep1)
	}
}

// A note is not a task, and the column stays empty. The activity table's own
// CHECK keeps assignee_id NULL on every other kind, so stamping one would fail
// the write rather than merely look wrong.
func TestANoteIsStoredWithNoAssignee(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	person := e.SeedPerson(t, "A Contact", &e.Rep1)

	note, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Subject: strPtr("They mentioned a rollout"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}

	if stored := storedAssignee(t, note.Id.String()); stored != nil {
		t.Fatalf("a note was stored against %v", *stored)
	}
}
