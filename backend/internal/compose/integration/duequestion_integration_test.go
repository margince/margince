// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/duequestion"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

var ownerPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}, "installation_settings": {Read: true}},
	RowScope: principal.RowScopeAll,
}

func fileExtensionTask(t *testing.T, e *Env, key string, assignee ids.UUID, due time.Time) error {
	t.Helper()
	ctx := principal.WithActor(connectorCtx(e, e.Rep1), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:ingress", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}}, RowScope: principal.RowScopeAll},
	})
	sink := capture.NewSink(e.DB()).WithClock(func() time.Time { return clocktest.Now(t) }).
		WithTaskAssigneeCheck(activities.CheckNewTaskAssignee).
		WithDueAsker(duequestion.Asker{Approvals: compose.NewApprovalsService(e.DB())})
	_, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: "unit", SourceID: key},
		Fields:     capture.ActivityFields{Kind: "task", Subject: "Send the quote", OccurredAt: clocktest.Now(t), AssigneeID: assignee, DueAt: due},
		Source:     "unit", CapturedBy: "connector:ingress",
	})
	return err
}

func dueQuestions(t *testing.T, e *Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'task_due_date'`)
}

func TestAFarDueDateOnACapturedTaskIsAskedOnceAndAnsweredByTheOwner(t *testing.T) {
	e := Setup(t)
	far := clocktest.Now(t).AddDate(15, 0, 0)
	for range 2 {
		if err := fileExtensionTask(t, e, "far", e.Rep1, far); err != nil {
			t.Fatalf("filing the task: %v", err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = 'far' AND due_at IS NULL`); n != 1 || dueQuestions(t, e) != 1 {
		t.Fatalf("a far date left %d undated task(s) and %d question(s), want 1 and 1", n, dueQuestions(t, e))
	}
	var id ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(), `SELECT id FROM approval WHERE kind = 'task_due_date'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, ownerPerms)
	svc := compose.NewApprovalsServiceWithEffects(e.Pool)
	if _, err := svc.Decide(owner, ids.From[ids.ApprovalKind](id), true, nil); err != nil {
		t.Fatalf("keeping the date: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity a JOIN audit_log l ON l.entity_id = a.id AND l.action = 'update' AND l.actor_id = 'human:' || $1
		WHERE a.source_id = 'far' AND a.due_at IS NOT NULL`, e.Rep1.String()); n == 0 {
		t.Error("the owner's answer did not write the due date through the audited update path")
	}
}

func TestADueDateInsideTheWindowOrWithNoOwnerAsksNobody(t *testing.T) {
	e := Setup(t)
	if err := fileExtensionTask(t, e, "near", e.Rep1, clocktest.Now(t).AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	if err := fileExtensionTask(t, e, "unowned", ids.Nil, clocktest.Now(t).AddDate(15, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = 'near' AND due_at IS NOT NULL`); n != 1 || dueQuestions(t, e) != 0 {
		t.Errorf("near task dated=%d, questions=%d; want 1 and 0", n, dueQuestions(t, e))
	}
}
