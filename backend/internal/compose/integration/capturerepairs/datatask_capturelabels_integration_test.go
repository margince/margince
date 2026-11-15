// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capturerepairs

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestOtherCaptureLabelsSurvivePagingRollbackAndReplay(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	var legacy []ids.UUID
	for range 3 {
		row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.UUID(row.Id)
		// The old classifier stored this label; the current writer emits other.
		e.WsExec(t, `UPDATE activity SET capture_label = 'noise', capture_labeled_at = now() WHERE id = $1`, id)
		legacy = append(legacy, id)
		got, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		if err != nil || got.CaptureLabel == nil || string(*got.CaptureLabel) != "other" {
			t.Fatalf("legacy public label = %v, err %v", got.CaptureLabel, err)
		}
	}
	restricted := legacy[2]
	legacy = legacy[:2]
	e.RestrictActivity(t, restricted)
	versions := e.WsScalar(t, `SELECT string_agg(id::text || ':' || version::text || ':' || updated_at::text, ',' ORDER BY id) FROM activity WHERE id = ANY($1)`, legacy)
	task := activities.CaptureLabelsDataTask()
	task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
	if count, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 1); err != nil || count != 1 || !capped {
		t.Fatalf("bounded estimate = %d, %t, %v", count, capped, err)
	}
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("interrupt after domain write")
	}
	e.RunDataTask(ctx, t, task)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND capture_label = 'noise'`, legacy); got != 2 {
		t.Fatalf("rollback kept %d legacy labels, want 2", got)
	}
	task.Steps[0].Batch = original
	// The failed run is resumed through the same executor after its backoff.
	e.WsExec(t, `UPDATE data_task_run SET next_attempt_at = NULL WHERE task_id = $1`, task.ID)
	e.RunDataTask(ctx, t, task)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id = ANY($1) AND capture_label = 'other'`, legacy); got != 2 {
		t.Fatalf("corrected %d labels, want 2", got)
	}
	if got := e.WsScalar(t, `SELECT string_agg(id::text || ':' || version::text || ':' || updated_at::text, ',' ORDER BY id) FROM activity WHERE id = ANY($1)`, legacy); got != versions {
		t.Error("label renaming changed edit versions or timestamps")
	}
	if got := e.WsScalar(t, `SELECT capture_label FROM activity WHERE id = $1`, restricted); got != "noise" {
		t.Error("restricted label changed")
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := original(ctx, tx, datatask.BatchIn{Limit: 100})
		if err != nil {
			return err
		}
		if out.Changed != 0 {
			t.Errorf("replay changed %d labels", out.Changed)
		}
		_, err = original(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte(`invalid`)})
		if err == nil {
			t.Error("malformed cursor was accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
