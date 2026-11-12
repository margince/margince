// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit filing a task for a colleague through Runtime.Ingest.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

var taskDue = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

func taskRecord(key string, assignee ids.UUID) extension.Record {
	return extension.Record{
		System: crmSystem,
		Key:    key,
		Activity: extension.ActivityFields{
			Kind:       extension.ActivityKindTask,
			Subject:    "Follow up on the stale deal",
			OccurredAt: time.Date(2026, 11, 1, 22, 0, 0, 0, time.UTC),
			AssigneeID: extension.UserID(assignee.String()),
			DueAt:      taskDue,
		},
	}
}

func (e *crmEnv) taskRows(t *testing.T, key string, assignee ids.UUID) int {
	t.Helper()
	return e.countAsWorkspace(t,
		`SELECT count(*) FROM activity WHERE kind = 'task' AND source_id = $1 AND assignee_id = $2 AND due_at = $3`,
		key, assignee, taskDue)
}

func TestATaskFiledForAColleagueIsTheirsAndAReplayLeavesItAlone(t *testing.T) {
	e := setupCRMIngress(t)
	e.mustIngest(t, taskRecord("remind-1", e.Rep2))
	if n := e.taskRows(t, "remind-1", e.Rep2); n != 1 {
		t.Fatalf("tasks assigned to the colleague = %d, want the one the unit filed", n)
	}

	// The unit re-runs and now names someone else. The task exists, so it is
	// not touched.
	again, err := e.ingest(t, e.member, taskRecord("remind-1", e.Rep1))
	if err != nil || again.Disposition != extension.DispositionAccepted {
		t.Fatalf("replay = %+v, %v, want accepted", again, err)
	}
	if n := e.taskRows(t, "remind-1", e.Rep2); n != 1 {
		t.Errorf("tasks still assigned to the first colleague = %d, want 1 after a replay", n)
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND source_id = $1`, "remind-1"); n != 1 {
		t.Errorf("task rows = %d after a replay, want one", n)
	}
}

func TestATaskForASeatThatCannotHoldWorkLandsNothing(t *testing.T) {
	e := setupCRMIngress(t)
	agent := ids.NewV7()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, is_agent) VALUES ($1, $2, 'Runner', true)`,
		agent, "runner-"+agent.String()+"@members.invalid"); err != nil {
		t.Fatalf("inserting the agent seat: %v", err)
	}

	for name, tc := range map[string]struct {
		assignee ids.UUID
		want     error
	}{
		"an agent seat":      {agent, extension.ErrInvalid},
		"a user not on file": {ids.NewV7(), extension.ErrNotFound},
	} {
		if _, err := e.ingest(t, e.member, taskRecord("remind-2", tc.assignee)); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if n := e.countAsWorkspace(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND source_id = $1`, "remind-2"); n != 0 {
		t.Errorf("task rows = %d after refused calls, want none", n)
	}
}
