// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedHomeRepairResumesItsExactPageAfterARolledBackTail(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -90)
	for range 3 {
		source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: new("Account discussion"), Source: "manual", OccurredAt: &old})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:repair-fixture"), activities.LogActivityInput{
			Kind: "task", Subject: new("Prepare the account review"), Source: "manual",
			SourceSystem: new(activities.CommitmentTaskSource), SourceID: new(ids.NewV7().String()), SourceActivityID: new(ids.UUID(source.Id)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	permissions := integration.AdminPerms
	permissions.Objects = maps.Clone(permissions.Objects)
	permissions.Objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
	operator := e.As(e.AdminUser, nil, permissions)
	e.GrantRole(t, e.AdminUser, "admin")
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,data_task}','{"read":true,"update":true,"create":false,"delete":false}'::jsonb) WHERE key='admin'`)
	var review datatask.ReviewRequest
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		first, err := previewHomeRepairAfterTx(operator, tx, ids.Nil, 2, now)
		if err != nil {
			return err
		}
		var scope homeRepairScope
		if err := json.Unmarshal(first.Scope, &scope); err != nil {
			return err
		}
		if len(scope.Tasks) != 2 {
			t.Fatalf("first page=%+v", scope)
		}
		tail, err := previewHomeRepairAfterTx(operator, tx, scope.Tasks[1].ID, 2, now)
		if err != nil {
			return err
		}
		var rest homeRepairScope
		if err := json.Unmarshal(tail.Scope, &rest); err != nil {
			return err
		}
		if len(rest.Tasks) != 1 || rest.Tasks[0].ID == scope.Tasks[1].ID {
			t.Fatalf("tail=%+v", rest)
		}
		scope.Tasks = append(scope.Tasks, rest.Tasks...)
		review, err = homeRepairReview(scope, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	task := homeRepairDataTask(e.Pool, func() time.Time { return now })
	task.Steps[0].Limit = 2
	newEngine := func(task datatask.Task) *datatask.Engine {
		engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		return engine
	}
	engine := newEngine(task)
	system := principal.SystemActing(operator, "system:home-repair-test")
	if err := engine.Seed(system); err != nil {
		t.Fatal(err)
	}
	if err := engine.Activate(operator, task.ID, task.Steps[0].Name, 1, review); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(system); err != nil {
		t.Fatal(err)
	}
	assertRepairProgress(t, engine, operator, 2, 2, datatask.Running)
	failing := task
	failing.Steps = append([]datatask.Step(nil), task.Steps...)
	batch := task.Steps[0].Batch
	failing.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("injected failure after the owning writer")
	}
	if _, err := newEngine(failing).Execute(system); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NOT NULL"); count != 2 {
		t.Fatalf("failed tail committed %d archives", count)
	}
	now = now.Add(time.Hour)
	resumed := newEngine(task)
	if _, err := resumed.Execute(system); err != nil {
		t.Fatal(err)
	}
	assertRepairProgress(t, resumed, operator, 3, 3, datatask.Done)
	if _, err := resumed.Execute(system); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND archived_at IS NOT NULL AND NOT is_done") != 3 {
		t.Fatal("resume lost a task or claimed fulfillment")
	}
	if e.WsCount(t, "SELECT count(*) FROM audit_log WHERE entity_type='activity' AND action='archive' AND evidence->>'rule_version'=$1", homeRepairVersion) != 3 {
		t.Fatal("resume duplicated or omitted an archive audit")
	}
}

func assertRepairProgress(t *testing.T, engine *datatask.Engine, ctx context.Context, scanned, changed int64, status string) {
	t.Helper()
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].Scanned != scanned || progress[0].Changed != changed || progress[0].Status != status {
		t.Fatalf("repair progress=%+v", progress)
	}
}
