// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func restoreLegacyRecipientThroughEngine(classifier context.Context, t *testing.T, e *integration.Env, activity ids.UUID, address string, at time.Time) {
	t.Helper()
	if err := database.WithWorkspaceTx(classifier, e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `DELETE FROM activity_participant WHERE activity_id=$1 AND address=$2`, activity, address); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `UPDATE activity SET raw_capture_id=NULL WHERE id=$1`, activity); err != nil {
			return err
		}
		task := mailRecipientsDataTask()
		out, err := task.Steps[0].Batch(classifier, tx, datatask.BatchIn{Limit: 50})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Fatalf("restored records=%d, want1", out.Changed)
		}
		replay, err := task.Steps[0].Batch(classifier, tx, datatask.BatchIn{Limit: 50})
		if err != nil {
			return err
		}
		if replay.Changed != 0 {
			t.Fatal("recipient recovery repeated a write")
		}
		var payload []byte
		if err := tx.QueryRow(context.Background(), `SELECT rc.payload FROM raw_capture rc JOIN activity a ON rc.source_system=a.source_system AND rc.source_id=a.source_id WHERE a.id=$1`, activity).Scan(&payload); err != nil {
			return err
		}
		if _, err := tx.Exec(context.Background(), `UPDATE raw_capture rc SET payload='{"encoding":"base64","data":"%%%%"}'::jsonb FROM activity a WHERE a.id=$1 AND rc.source_system=a.source_system AND rc.source_id=a.source_id`, activity); err != nil {
			return err
		}
		if out, err := task.Steps[0].Batch(classifier, tx, datatask.BatchIn{Limit: 50}); err == nil || out.Done {
			t.Fatal("unreadable original reported a completed correction")
		}
		if _, err := tx.Exec(context.Background(), `UPDATE raw_capture rc SET payload=$2::jsonb FROM activity a WHERE a.id=$1 AND rc.source_system=a.source_system AND rc.source_id=a.source_id`, activity, payload); err != nil {
			return err
		}

		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(classifier, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(classifier, `DELETE FROM activity_participant WHERE activity_id=$1 AND address=$2`, activity, address)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	engine, err := newDataTaskEngine(e.Pool, "test", func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(classifier); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if _, err := engine.Execute(classifier); err != nil {
			t.Fatal(err)
		}
	}
	progress, err := engine.List(classifier)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range progress {
		if step.TaskID == mailRecipientsDataTask().ID {
			if step.Status != datatask.Done || step.Changed != 1 || step.Batches != 1 {
				t.Fatalf("recipient repair progress=%+v", step)
			}
			return
		}
	}
	t.Fatal("recipient correction was not registered in the engine")
}
