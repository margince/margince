// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The lead qualify task as its owner meets it. A held meeting with a lead asks
// the lead's owner to qualify them; the suites that drive the writers that mark
// a meeting held check the result here, so each reads the same surfaces.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// QualifyTask is one open qualify task on a lead, as stored.
type QualifyTask struct {
	ID             ids.UUID
	Subject        string
	Assignee       *ids.UUID
	SourceActivity *ids.UUID
}

// OpenQualifyTasks lists the open qualify tasks on a lead, whoever holds them.
func OpenQualifyTasks(ctx context.Context, t *testing.T, db *database.DB, lead ids.UUID) []QualifyTask {
	t.Helper()
	var tasks []QualifyTask
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT a.id, a.subject, a.assignee_id, a.source_activity_id
			  FROM activity a JOIN activity_link al ON al.activity_id = a.id
			 WHERE al.lead_id = $1 AND a.kind = 'task' AND a.source_system = $2
			   AND NOT a.is_done AND a.archived_at IS NULL ORDER BY a.id`, lead, provenance.LeadRepliedSource)
		if err != nil {
			return err
		}
		tasks, err = pgx.CollectRows(rows, pgx.RowToStructByPos[QualifyTask])
		return err
	})
	if err != nil {
		t.Fatalf("reading the lead's qualify tasks: %v", err)
	}
	return tasks
}

// AssertOwnerAskedToQualify checks that the held meeting filed one open qualify
// task on the lead, assigned to the owner owner acts as, and that the owner
// meets it where the product shows it: on the Worklist the route assembles, and
// as the lead's next task in its open-task count.
func AssertOwnerAskedToQualify(owner context.Context, t *testing.T, db *database.DB, lead, meeting ids.UUID) {
	t.Helper()
	actor, ok := principal.Actor(owner)
	if !ok {
		t.Fatal("the owner's context carries no actor")
	}
	tasks := OpenQualifyTasks(owner, t, db, lead)
	if len(tasks) != 1 {
		t.Fatalf("the lead holds %d open qualify tasks, want 1: %+v", len(tasks), tasks)
	}
	task := tasks[0]
	if task.Assignee == nil || *task.Assignee != actor.UserID {
		t.Errorf("the qualify task is assigned to %v, want the lead's owner %s", task.Assignee, actor.UserID)
	}
	if task.SourceActivity == nil || *task.SourceActivity != meeting {
		t.Errorf("the qualify task's source is %v, want the held meeting %s", task.SourceActivity, meeting)
	}

	day, err := compose.NewAttentionService(db.Pool(), approvals.NewService(db), time.Now).
		Worklist(owner, "mine", "all", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatalf("reading the owner's Worklist: %v", err)
	}
	shown := false
	for _, item := range day.Queue {
		shown = shown || (item.Source == "task" && item.Id == task.ID.String())
	}
	if !shown {
		t.Errorf("the owner's Worklist does not show the qualify task %s (%d rows)", task.ID, len(day.Queue))
	}

	got, err := contacts.NewStore(db).GetLead(owner, ids.From[ids.LeadKind](lead), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the lead: %v", err)
	}
	if got.OpenTaskCount == nil || *got.OpenTaskCount != 1 {
		t.Errorf("the lead's open_task_count = %v, want 1", got.OpenTaskCount)
	}
	if got.NextTaskSubject == nil || *got.NextTaskSubject != task.Subject {
		t.Errorf("the lead's next task = %v, want %q", got.NextTaskSubject, task.Subject)
	}
}
