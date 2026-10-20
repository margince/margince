// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedDataTaskRequiresActivationAndPreservesItsPreviousRun(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	task := datatask.Task{ID: "core/100_reviewed", Version: 1, Title: "Reviewed correction", Manual: true, ReviewVersion: "rules-1",
		Review: func(context.Context, pgx.Tx, datatask.ReviewRequest) (string, error) { return "reviewed-digest", nil },
		Steps: []datatask.Step{{Name: "repair", Limit: 2, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(context.Context, datatask.Querier, int64) (int64, bool, error) { return 10, false, nil },
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				if in.Activation == nil || in.Activation.Operator != "human:"+e.AdminUser.String() {
					return datatask.BatchOut{}, apperrors.ErrPermissionDenied
				}
				count := 0
				if len(in.Cursor) > 0 {
					if err := json.Unmarshal(in.Cursor, &count); err != nil {
						return datatask.BatchOut{}, err
					}
				}
				for range in.Limit {
					if _, err := storekit.LogSystem(ctx, tx, "data_task.reviewed_probe", map[string]any{"run": in.RunID}); err != nil {
						return datatask.BatchOut{}, err
					}
				}
				cursor, err := json.Marshal(count + in.Limit)
				return datatask.BatchOut{Cursor: cursor, Scanned: in.Limit, Changed: in.Limit}, err
			},
		}},
	}
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(e.Admin(), "system:reviewed-task-test")
	if err := engine.Seed(system); err != nil {
		t.Fatal(err)
	}
	if out, err := engine.Execute(system); err != nil || out.More {
		t.Fatalf("unactivated work ran: %+v %v", out, err)
	}
	review := datatask.ReviewRequest{Scope: json.RawMessage(`{"ids":[]}`), Digest: "reviewed-digest", RuleVersion: "rules-1", Limit: 3, AsOf: now}
	if err := engine.Activate(system, task.ID, "repair", 1, review); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("system activated a manual review: %v", err)
	}
	if err := engine.Activate(dataTaskReader(e), task.ID, "repair", 1, review); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := engine.Execute(system); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := engine.List(dataTaskReader(e))
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 1 || progress[0].Status != datatask.Done || progress[0].Scanned != 3 || progress[0].Batches != 2 {
		t.Fatalf("bounded progress: %+v", progress)
	}
	if e.WsCount(t, "SELECT count(*) FROM system_log WHERE action='data_task.reviewed_probe'") != 3 {
		t.Fatal("reviewed bound did not constrain writes")
	}
	if err := engine.Activate(dataTaskReader(e), task.ID, "repair", 1, review); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale activation: %v", err)
	}
	if err := engine.Activate(dataTaskReader(e), task.ID, "repair", progress[0].Version, review); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, "SELECT count(*) FROM data_task_run_history WHERE task_id='core/100_reviewed' AND progress->>'status'='done'") != 1 {
		t.Fatal("new activation lost prior run")
	}
}
