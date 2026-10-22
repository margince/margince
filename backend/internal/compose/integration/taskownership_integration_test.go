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
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
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
	contact := e.SeedContact(t, "A Contact", &e.Rep1)

	task, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "task", Subject: new("Call them back"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
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

// A rep's AI assistant that files a follow-up over MCP, naming nobody, files it
// for the rep. The task lands on the rep's own Worklist, which reads assigned
// work only.
func TestATaskAnAgentFilesForARepReachesThatRepsWorklist(t *testing.T) {
	d := newTwoDoors(t, "agent-task-owner", "read", "write")
	var me struct {
		User struct {
			ID ids.UUID `json:"id"`
		} `json:"user"`
	}
	if status := d.e.Call(t, http.MethodGet, "/v1/me", nil, nil, &me); status != http.StatusOK || me.User.ID.IsZero() {
		t.Fatalf("reading the rep's own identity → %d", status)
	}

	filed := d.tool(t, "create_task", map[string]any{
		"subject": "Follow up with Contact A",
		"due_at":  time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
	})
	task := anIDIn(t, filed)

	stored := readAssignee(t, d.e, task)
	if stored == nil || *stored != me.User.ID {
		t.Fatalf("the task the agent filed for %v was stored against %v", me.User.ID, stored)
	}
	if !onWorklist(t, d.e, "mine", task) {
		t.Fatal("the task the agent filed for the rep is missing from the rep's Mine queue")
	}
	if onWorklist(t, d.e, "unassigned", task) {
		t.Fatal("the task the agent filed for the rep still sits in the Unassigned queue")
	}
}

// readAssignee reads the task back through its own endpoint, apart from the
// answer the tool gave.
func readAssignee(t *testing.T, e *apptest.AppEnv, id string) *ids.UUID {
	t.Helper()
	var task struct {
		AssigneeID *ids.UUID `json:"assignee_id"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/activities/"+id, nil, nil, &task); status != http.StatusOK {
		t.Fatalf("reading the task back → %d", status)
	}
	return task.AssigneeID
}

// onWorklist answers whether the signed-in reader's Worklist under scope
// carries the record.
func onWorklist(t *testing.T, e *apptest.AppEnv, scope, id string) bool {
	t.Helper()
	var page struct {
		Queue []struct {
			ID string `json:"id"`
		} `json:"queue"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/worklist?scope="+scope+"&filter=all", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("reading the %s Worklist → %d", scope, status)
	}
	for _, row := range page.Queue {
		if row.ID == id {
			return true
		}
	}
	return false
}

// A note is not a task, and the column stays empty. The activity table's own
// CHECK keeps assignee_id NULL on every other kind, so stamping one would fail
// the write rather than merely look wrong.
func TestANoteIsStoredWithNoAssignee(t *testing.T) {
	e := Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	contact := e.SeedContact(t, "A Contact", &e.Rep1)

	note, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Subject: new("They mentioned a rollout"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}

	if stored := storedAssignee(t, note.Id.String()); stored != nil {
		t.Fatalf("a note was stored against %v", *stored)
	}
}
