// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package qualifytest checks the lead qualify task as its owner meets it. A
// held meeting with a lead asks the lead's owner to qualify them. Every suite
// that drives a writer marking a meeting held checks the result here.
package qualifytest

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Task is one open qualify task on a lead, as stored.
type Task struct {
	ID             ids.UUID
	Subject        string
	Assignee       *ids.UUID
	SourceActivity *ids.UUID
	SourceID       string
}

// OpenTasks lists the open qualify tasks on a lead that reader can see.
func OpenTasks(reader context.Context, t *testing.T, db *database.DB, lead ids.UUID) []Task {
	t.Helper()
	rows, _, err := activities.NewStore(db).ListActivities(reader, activities.ListActivitiesInput{
		EntityType: new("lead"), EntityID: &lead, Kind: new("task"), SourceSystem: new(provenance.LeadRepliedSource),
	})
	if err != nil {
		t.Fatalf("reading the lead's qualify tasks: %v", err)
	}
	var tasks []Task
	for _, row := range rows {
		if row.IsDone != nil && *row.IsDone {
			continue
		}
		task := Task{ID: ids.UUID(row.Id), Assignee: (*ids.UUID)(row.AssigneeId), SourceActivity: (*ids.UUID)(row.SourceActivityId)}
		if row.Subject != nil {
			task.Subject = *row.Subject
		}
		if row.SourceId != nil {
			task.SourceID = *row.SourceId
		}
		tasks = append(tasks, task)
	}
	return tasks
}

// AssertOwnerAsked checks that the held meeting filed one open qualify task on
// the lead, assigned to the owner and worded for a held meeting. The owner must
// also meet it on the Worklist and as the lead's next task.
func AssertOwnerAsked(owner context.Context, t *testing.T, db *database.DB, lead, meeting ids.UUID) {
	t.Helper()
	tasks := OpenTasks(owner, t, db, lead)
	if len(tasks) != 1 {
		t.Fatalf("the lead holds %d open qualify tasks, want 1: %+v", len(tasks), tasks)
	}
	got, err := contacts.NewStore(db).GetLead(owner, ids.From[ids.LeadKind](lead), storekit.LiveOnly)
	if err != nil || got.FullName == nil {
		t.Fatalf("reading the lead's name: %v", err)
	}
	subject := fmt.Sprintf(systemtext.Copy("lead.held", textlang.English), *got.FullName)
	assertStored(owner, t, tasks[0], lead, meeting, subject)
	assertOnWorklist(owner, t, db, tasks[0])
	if got.OpenTaskCount == nil || *got.OpenTaskCount != 1 {
		t.Errorf("the lead's open_task_count = %v, want 1", got.OpenTaskCount)
	}
	if got.NextTaskSubject == nil || *got.NextTaskSubject != subject {
		t.Errorf("the lead's next task = %v, want %q", got.NextTaskSubject, subject)
	}
}

// assertStored checks who holds the task, what it reads and which meeting it answers.
func assertStored(owner context.Context, t *testing.T, task Task, lead, meeting ids.UUID, subject string) {
	t.Helper()
	actor, ok := principal.Actor(owner)
	if !ok {
		t.Fatal("the owner's context carries no actor")
	}
	if task.Assignee == nil || *task.Assignee != actor.UserID {
		t.Errorf("the qualify task is assigned to %v, want the lead's owner %s", task.Assignee, actor.UserID)
	}
	if task.SourceActivity == nil || *task.SourceActivity != meeting {
		t.Errorf("the qualify task's source is %v, want the held meeting %s", task.SourceActivity, meeting)
	}
	if want := provenance.LeadQualifyKey(meeting, lead); task.SourceID != want {
		t.Errorf("the qualify task's source_id = %q, want %q", task.SourceID, want)
	}
	if task.Subject != subject {
		t.Errorf("the qualify task reads %q, want %q", task.Subject, subject)
	}
}

// assertOnWorklist reads the owner's Worklist through the wiring the route serves.
func assertOnWorklist(owner context.Context, t *testing.T, db *database.DB, task Task) {
	t.Helper()
	at := clocktest.Now(t)
	day, err := compose.NewAttentionService(db.Pool(), compose.NewApprovalsService(db), func() time.Time { return at }).
		Worklist(owner, "mine", "all", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatalf("reading the owner's Worklist: %v", err)
	}
	for _, item := range day.Queue {
		if item.Source == "task" && item.Id == task.ID.String() {
			return
		}
	}
	t.Errorf("the owner's Worklist does not show the qualify task %s (%d rows)", task.ID, len(day.Queue))
}
