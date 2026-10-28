// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReviewedLegacyCorrectionsResumeThroughTheDataTaskEngine(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	for range 3 {
		seedLegacyRepairOn(t, e, now, false)
	}
	operator := repairOperator(t, e)
	var review datatask.ReviewRequest
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		var err error
		review, err = previewHomeRepairAfterTx(operator, tx, ids.Nil, 100, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if review.Limit != 3 {
		t.Fatalf("preview included %d items, want three", review.Limit)
	}
	task := homeRepairDataTask(e.Pool, func() time.Time { return now })
	task.Steps[0].Limit = 2
	system, engine := probeEngine(t, e, &now, task)
	if err := engine.Activate(operator, task.ID, task.Steps[0].Name, 1, review); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(system); err != nil {
		t.Fatal(err)
	}
	assertRepairProgress(operator, t, engine, 2, 2, datatask.Running)
	failing := task
	failing.Steps = append([]datatask.Step(nil), task.Steps...)
	batch := task.Steps[0].Batch
	failing.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := batch(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("rollback after the owning writer")
	}
	_, failed := probeEngine(t, e, &now, failing)
	if _, err := failed.Execute(system); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, "SELECT count(*) FROM audit_log WHERE entity_type='activity' AND evidence->>'automatic_task_repair'='1'"); got != 2 {
		t.Fatalf("rolled-back tail committed %d corrections", got)
	}
	now = now.Add(time.Hour)
	_, resumed := probeEngine(t, e, &now, task)
	if _, err := resumed.Execute(system); err != nil {
		t.Fatal(err)
	}
	assertRepairProgress(operator, t, resumed, 3, 3, datatask.Done)
	if _, err := resumed.Execute(system); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o
 ON o.envelope->'trace'->>'audit_log_id'=a.id::text
 WHERE a.entity_type='activity' AND a.action='update' AND a.actor_type='human'
 AND o.envelope->'payload'->'changed_fields'->>'automatic_task_repaired'='true'
 AND a.evidence->>'automatic_task_repair'='1'`); got != 3 {
		t.Fatalf("resumed corrections lost their exact audit/outbox trace: %d", got)
	}
	if e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task' AND source_activity_id IS NOT NULL AND due_at IS NULL AND NOT is_done AND archived_at IS NULL") != 3 {
		t.Fatal("resume changed work or lost a correction")
	}
}
