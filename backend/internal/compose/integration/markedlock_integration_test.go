// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/privacyreview"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	markedTaskID     = "core/1_marked_audit_correction"
	markedWeeklyStep = "marked_weekly_images"
	markedReqStep    = "retired_request_images"
)

func markedTask() datatask.Task {
	weekly := privacyreview.AuditSteps(privacy.RedactMarkedWeeklyAuditPageTx)[0]
	weekly.Name, weekly.Limit = markedWeeklyStep, privacy.MarkerAuditPageSize
	requests := privacyreview.AuditSteps(privacy.RedactRetiredRequestAuditPageTx)[0]
	requests.Name, requests.Limit = markedReqStep, privacy.MarkerAuditPageSize
	return datatask.Task{ID: markedTaskID, Version: 1, Title: "Marked audit correction", Steps: []datatask.Step{weekly, requests}}
}

func markedWeeklyCopies(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	plan, commitment := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO weekly_plan(id,owner_id,local_week_start,captured_by) VALUES(@id,@owner,date '2026-11-02','system:legacy') ON CONFLICT(owner_id,local_week_start) DO NOTHING`, pgx.StrictNamedArgs{"id": plan, "owner": e.AdminUser})
	e.WsExec(t, `INSERT INTO weekly_plan_commitment(id,plan_id,label,help_requested,due_on,position,captured_by) SELECT @id,p.id,'(erased: the contact this named exercised erasure)','',date '2026-11-06',0,'system:legacy' FROM weekly_plan p WHERE p.owner_id=@owner AND p.local_week_start=date '2026-11-02'`, pgx.StrictNamedArgs{"id": commitment, "owner": e.AdminUser})
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.AuditWithEvidence(e.Admin(), tx, "create", "weekly_plan_commitment", commitment, nil, map[string]any{"label": "Private Contact"}, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return commitment
}

func markedRequestCopies(t *testing.T, e *Env) ids.UUID {
	t.Helper()
	subject, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Erased Subject", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	request, received := ids.NewV7(), clocktest.Now(t)
	e.WsExec(t, `INSERT INTO data_subject_request(id,kind,subject_ref,status,resolution,received_at,due_at,channel,receipt_reference)
 VALUES(@id,'access','erased+'||@id::uuid::text,'fulfilled','erased',@received,@due,'phone',@receipt)`, pgx.StrictNamedArgs{
		"id": request, "receipt": "RCPT-" + request.String(), "received": received, "due": received.AddDate(0, 0, 30),
	})
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		image := map[string]any{"subject_ref": subject.Id.String(), "email": "private@example.test", "status": "fulfilled"}
		_, err := storekit.AuditWithEvidence(e.Admin(), tx, "create", "data_subject_request", request, nil, image, nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return request
}

func markedImages(t *testing.T, e *Env, weekly, request ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT (SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND after->>'label'='Private Contact')
 + (SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=$2 AND after->>'email'='private@example.test')`, weekly, request)
}

func stepRows(t *testing.T, e *Env, step, condition string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM data_task_run WHERE task_id=$1 AND step=$2 AND `+condition, markedTaskID, step)
}

func TestMarkedStepsStayUnfinishedWhileALockIsBusyAndFinishOnceItIsFree(t *testing.T) {
	e := Setup(t)
	weekly, request := markedWeeklyCopies(t, e), markedRequestCopies(t, e)
	busy := e.SeedContact(t, "Busy Contact", nil)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{markedTask()}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	finish := func(step string) {
		for range 4 {
			if stepRows(t, e, step, `status='done'`) == 1 {
				return
			}
			if _, err := engine.Execute(ctx); err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Hour)
		}
	}
	busyThenFree := func(step, hold string, arg any) {
		holder, err := e.Pool.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Error(err)
			}
		}()
		if _, err := holder.Exec(context.Background(), hold, arg); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		if n := stepRows(t, e, step, `status='running' AND cursor IS NULL AND scanned=0 AND consecutive_failures>=1 AND next_attempt_at IS NOT NULL`); n != 1 {
			t.Fatalf("%s: a busy lock must leave the cursor in place and schedule a retry", step)
		}
		now = now.Add(time.Hour)
	}
	busyThenFree(markedWeeklyStep, `UPDATE contact SET legal_hold=false WHERE id=$1`, busy)
	busyThenFree(markedWeeklyStep, `SELECT 1 FROM weekly_plan_commitment WHERE id=$1 FOR UPDATE`, weekly)
	if n := markedImages(t, e, weekly, request); n != 2 {
		t.Fatalf("a busy lock changed the history: %d images left", n)
	}
	finish(markedWeeklyStep)
	if stepRows(t, e, markedWeeklyStep, `status='done'`) != 1 {
		t.Fatal("the weekly step did not finish once the locks were free")
	}
	busyThenFree(markedReqStep, `SELECT 1 FROM data_subject_request WHERE id=$1 FOR UPDATE`, request)
	finish(markedReqStep)
	if stepRows(t, e, markedReqStep, `status='done'`) != 1 {
		t.Fatal("the request step did not finish once the locks were free")
	}
	if n := markedImages(t, e, weekly, request); n != 0 {
		t.Fatalf("%d old personal images are left", n)
	}
}

func TestRetentionGoesOnWhenAMarkedCorrectionLockIsBusyAndCorrectsOnTheNextRun(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	weekly, request := markedWeeklyCopies(t, e), markedRequestCopies(t, e)
	old, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Old News", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	var aged ids.UUID
	monthAgo := clocktest.Now(t).AddDate(0, 0, -31)
	if err := owner.QueryRow(context.Background(), `UPDATE event_outbox SET published_at=$2, created_at=$2
 WHERE id=(SELECT id FROM event_outbox WHERE envelope->'entity'->>'id'=$1::text ORDER BY id DESC LIMIT 1) RETURNING id`, old.Id.String(), monthAgo).Scan(&aged); err != nil {
		t.Fatal(err)
	}
	busy := e.SeedContact(t, "Busy Contact", nil)
	service := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	holder, err := e.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := holder.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	if _, err := holder.Exec(context.Background(), `UPDATE contact SET legal_hold=false WHERE id=$1`, busy); err != nil {
		t.Fatal(err)
	}
	if err := service.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatalf("a busy correction lock failed the retention run: %v", err)
	}
	if e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE id=$1`, aged) != 0 {
		t.Error("a later retention stage did not run")
	}
	if n := markedImages(t, e, weekly, request); n != 2 {
		t.Fatalf("the busy run changed the history: %d images left", n)
	}
	if err := holder.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	if n := markedImages(t, e, weekly, request); n != 0 {
		t.Fatalf("the next run left %d old personal images", n)
	}
}

func TestAHeldRecordKeepsItsCopyAndTheNextRetentionPassCorrectsItOnceTheHoldLifts(t *testing.T) {
	for _, source := range []struct {
		name  string
		seed  func(*testing.T, *Env) ids.UUID
		count string
	}{
		{"weekly image", markedWeeklyCopies, `SELECT count(*) FROM audit_log WHERE entity_type='weekly_plan_commitment' AND entity_id=$1 AND after->>'label'='Private Contact'`},
		{"retired request", markedRequestCopies, `SELECT count(*) FROM audit_log WHERE entity_type='data_subject_request' AND entity_id=$1 AND after->>'email'='private@example.test'`},
	} {
		t.Run(source.name, func(t *testing.T) {
			e := Setup(t)
			record := source.seed(t, e)
			held := e.SeedContact(t, "Held Contact", nil)
			e.WsExec(t, `UPDATE contact SET legal_hold=true WHERE id=$1`, held)
			ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
			now := clocktest.Now(t)
			engine, err := datatask.New(e.Pool, []datatask.Task{markedTask()}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if err := engine.Seed(ctx); err != nil {
				t.Fatal(err)
			}
			for range 6 {
				if _, err := engine.Execute(ctx); err != nil {
					t.Fatal(err)
				}
				now = now.Add(time.Hour)
			}
			if n := e.WsCount(t, source.count, record); n == 0 {
				t.Fatal("the correction removed a copy while a legal hold stood")
			}
			e.WsExec(t, `UPDATE contact SET legal_hold=false WHERE id=$1`, held)
			service := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err := service.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
				t.Fatal(err)
			}
			if n := e.WsCount(t, source.count, record); n != 0 {
				t.Fatalf("the next retention pass left %d copies after the hold lifted", n)
			}
		})
	}
}
