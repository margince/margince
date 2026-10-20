// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCombinedActivityAndFollowUpCommitOnce(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	sourceSystem, sourceID := "manual", "review-with-follow-up"
	in := LogActivityInput{
		Kind: "note", Subject: new("Review"), Source: "manual", SourceSystem: &sourceSystem, SourceID: &sourceID,
		FollowUpTask: &LogActivityInput{Kind: "task", Subject: new("Send pricing"), Source: "manual"},
	}
	var source ids.ActivityID
	for pass := range 2 {
		row, created, err := store.LogActivity(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		if created != (pass == 0) {
			t.Fatalf("pass %d created=%t", pass, created)
		}
		source = ids.From[ids.ActivityKind](ids.UUID(row.Id))
	}
	tasks, page, err := store.ListActivities(ctx, ListActivitiesInput{SourceActivityID: &source})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || page.HasMore || tasks[0].SourceActivityId == nil || ids.UUID(*tasks[0].SourceActivityId) != source.UUID {
		t.Fatalf("follow-ups=%+v page=%+v", tasks, page)
	}
	if tasks[0].Subject == nil || *tasks[0].Subject != "Send pricing" {
		t.Fatal("follow-up lost its requested work")
	}
	var activities, audits, events int
	if err := e.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM activity),(SELECT count(*) FROM audit_log WHERE entity_type='activity' AND action='create'),(SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.captured')`).Scan(&activities, &audits, &events); err != nil {
		t.Fatal(err)
	}
	if activities != 2 || audits != 2 || events != 2 {
		t.Fatalf("activity/audit/event counts=%d/%d/%d", activities, audits, events)
	}
	if _, err := store.GetActivity(ctx, source, storekit.LiveOnly); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidFollowUpRollsBackItsSourceAndTrace(t *testing.T) {
	e := setupSend(t)
	missing := ids.New[ids.UserKind]()
	_, _, err := e.store(nil).LogActivity(e.as(principal.RowScopeAll), LogActivityInput{
		Kind: "note", Subject: new("Review"), Source: "manual",
		FollowUpTask: &LogActivityInput{Kind: "task", Subject: new("Send pricing"), Source: "manual", AssigneeID: &missing},
	})
	if err == nil {
		t.Fatal("invalid follow-up was accepted")
	}
	var activities, audits, events int
	if err := e.owner.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM activity),(SELECT count(*) FROM audit_log WHERE entity_type='activity'),(SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.captured')`).Scan(&activities, &audits, &events); err != nil {
		t.Fatal(err)
	}
	if activities != 0 || audits != 0 || events != 0 {
		t.Fatalf("rolled-back activity/audit/event counts=%d/%d/%d", activities, audits, events)
	}
}

func TestFollowUpContentTracksItsSourceAudience(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	source := loggedNote(ctx, t, e)
	sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
	task, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Discuss the private plan"), Source: "manual", SourceActivityID: &sourceID.UUID})
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	colleague := principal.WithActor(ctx, actor)
	taskID := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	before, err := store.GetActivity(colleague, taskID, storekit.LiveOnly)
	if err != nil || before.Subject == nil {
		t.Fatalf("shared task unavailable: %+v %v", before, err)
	}
	if _, err := store.SetAudience(ctx, sourceID, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	after, err := store.GetActivity(colleague, taskID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if after.Subject != nil || after.Body != nil {
		t.Fatal("task disclosed content after its source became private")
	}
	own, err := store.GetActivity(ctx, taskID, storekit.LiveOnly)
	if err != nil || own.Subject == nil {
		t.Fatalf("source reader lost task: %+v %v", own, err)
	}
	if _, _, err := store.ListActivities(colleague, ListActivitiesInput{SourceActivityID: &sourceID}); err == nil {
		t.Fatal("reverse list accepted a withheld source")
	}
	if _, _, err := store.LogActivity(colleague, LogActivityInput{Kind: "task", Subject: new("Hidden source"), Source: "manual", SourceActivityID: &sourceID.UUID}); err == nil {
		t.Fatal("task accepted a withheld source")
	}
}

func TestFollowUpCannotUseAnotherTaskAsItsSource(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	task, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Original"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(task.Id)
	if _, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Nested"), Source: "manual", SourceActivityID: &source}); err == nil {
		t.Fatal("task-to-task source accepted")
	}
}

func TestFollowUpRefusesASourceOutsideTheInstallation(t *testing.T) {
	e := setupSend(t)
	source := ids.NewV7()
	_, _, err := e.store(nil).LogActivity(e.as(principal.RowScopeAll), LogActivityInput{
		Kind: "task", Subject: new("Send scope"), Source: "manual", SourceActivityID: &source,
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("unknown source = %v, want not found", err)
	}
	var count int
	if err := e.owner.QueryRow(context.Background(), "SELECT count(*) FROM activity").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("unresolved source left a task behind")
	}
}

func TestFollowUpRefusesAnAssigneeOutsideItsSourceAudience(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	source := loggedNote(ctx, t, e)
	sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
	if _, err := store.SetAudience(ctx, sourceID, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	other := ids.From[ids.UserKind](e.other)
	_, _, err := store.LogActivity(ctx, LogActivityInput{Kind: "task", Subject: new("Read private notes"), Source: "manual", SourceActivityID: &sourceID.UUID, AssigneeID: &other})
	var field *followUpFieldError
	if !errors.As(err, &field) || field.Field != fieldAssignee {
		t.Fatalf("inaccessible assignment error = %v", err)
	}
	tasks, _, err := store.ListActivities(ctx, ListActivitiesInput{SourceActivityID: &sourceID})
	if err != nil || len(tasks) != 0 {
		t.Fatalf("refused assignment left tasks: %+v %v", tasks, err)
	}
}

func TestCombinedHTTPFollowUpAcceptsAnExplicitInvitedAssignee(t *testing.T) {
	e := setupSend(t)
	ctx, store := e.as(principal.RowScopeAll), e.store(nil)
	args := []any{e.other}
	if _, err := e.owner.Exec(ctx, storekit.SQLf("UPDATE app_user SET status='invited' WHERE id=$%d", len(args)), args...); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"kind":"note","subject":"Review","source":"manual","follow_up_task":{"subject":"Send pricing","assignee_id":%q}}`, e.other.String())
	req := httptest.NewRequest(http.MethodPost, "/v1/activities", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	Handlers{store: store}.LogActivity(response, req, crmcontracts.LogActivityParams{})
	if response.Code != http.StatusCreated {
		t.Fatalf("combined write: %d %s", response.Code, response.Body.String())
	}
	var source crmcontracts.Activity
	if err := json.Unmarshal(response.Body.Bytes(), &source); err != nil {
		t.Fatal(err)
	}
	sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
	tasks, _, err := store.ListActivities(ctx, ListActivitiesInput{SourceActivityID: &sourceID})
	if err != nil || len(tasks) != 1 || tasks[0].AssigneeId == nil || ids.UUID(*tasks[0].AssigneeId) != e.other {
		t.Fatalf("assigned follow-up: %+v %v", tasks, err)
	}
}
