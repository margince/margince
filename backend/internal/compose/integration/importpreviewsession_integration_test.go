// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// preview_import works on the run its stored source opened. A test advances
// that draft, and a refusal before the test cancels it. A source name still
// being written answers with the run writing it.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// storedRun is what the database holds for one csv run.
type storedRun struct {
	ID, Status, SourceRef string
	Released              bool
}

func storedImportRuns(t *testing.T, e *apptest.AppEnv) []storedRun {
	t.Helper()
	var runs []storedRun
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT id::text, status, source_ref, source_released_at IS NOT NULL
			  FROM import_run WHERE connector = 'csv' ORDER BY created_at, id`)
		if err != nil {
			return err
		}
		runs, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (storedRun, error) {
			var r storedRun
			return r, row.Scan(&r.ID, &r.Status, &r.SourceRef, &r.Released)
		})
		return err
	}); err != nil {
		t.Fatalf("reading the runs: %v", err)
	}
	return runs
}

type previewAnswer struct {
	Run struct {
		RunID string `json:"run_id"`
		State string `json:"state"`
	} `json:"run"`
}

func previewCompanies(t *testing.T, d *twoDoors, sourceName string) previewAnswer {
	t.Helper()
	var preview previewAnswer
	decode(t, d.tool(t, "preview_import", map[string]any{
		"object": "company", "csv": "name\nPreviewby GmbH\n",
		"mapping": map[string]string{"name": "display_name"}, "source_name": sourceName,
	}), &preview)
	if preview.Run.State != "awaiting_approval" {
		t.Fatalf("the preview answered %+v, want a run awaiting approval", preview)
	}
	return preview
}

func TestAPreviewOfAStoredSourceAdvancesItsDraft(t *testing.T) {
	d := newTwoDoors(t, "preview-advances", "read", "write")
	preview := previewCompanies(t, d, "")
	runs := storedImportRuns(t, d.e)
	if len(runs) != 1 || runs[0].ID != preview.Run.RunID {
		t.Fatalf("runs = %+v, want only the draft the stored source opened, %s", runs, preview.Run.RunID)
	}

	var retested importRunDTO
	if status := d.e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": "company", "source_ref": runs[0].SourceRef,
		"mapping": map[string]string{"name": "display_name"}, "on_duplicate": "skip",
	}, d.bearer, &retested); status != http.StatusAccepted || retested.ID != preview.Run.RunID {
		t.Fatalf("a second test of the stored source → %d %+v, want 202 on run %s", status, retested, preview.Run.RunID)
	}
	if again := storedImportRuns(t, d.e); len(again) != 1 {
		t.Fatalf("runs = %+v, want the one draft, tested twice", again)
	}
}

func TestAFailedPreviewCancelsItsDraft(t *testing.T) {
	d := newTwoDoors(t, "preview-cancels", "read", "write")
	refusal := d.toolRefused(t, "preview_import", map[string]any{
		"object": "company", "csv": "id,Company\nx,Acme\n",
	})
	if !strings.Contains(refusal, "Company") {
		t.Fatalf("the refusal = %q, want it to name the column it could not place", refusal)
	}
	runs := storedImportRuns(t, d.e)
	if len(runs) != 1 || runs[0].Status != "cancelled" || !runs[0].Released {
		t.Fatalf("runs = %+v, want the call's draft cancelled with its file released", runs)
	}
}

func TestABusySourceAnswersTheOtherRunsID(t *testing.T) {
	// The commit job is never worked, so the first run stays writing.
	d := newTwoDoorsWith(t, "preview-busy", nil, "read", "write")
	writing := previewCompanies(t, d, "Fair 2026")
	d.tool(t, "commit_import", map[string]any{"run_id": writing.Run.RunID})

	refusal := d.toolRefused(t, "preview_import", map[string]any{
		"object": "company", "csv": "name\nSecond GmbH\n",
		"mapping": map[string]string{"name": "display_name"}, "source_name": "Fair 2026",
	})
	if !strings.Contains(refusal, writing.Run.RunID) || !strings.Contains(refusal, "read_import that run") {
		t.Errorf("the refusal = %q, want it to send the caller to read_import run %s", refusal, writing.Run.RunID)
	}
	if !strings.Contains(refusal, "can succeed later") {
		t.Errorf("the refusal = %q, want it to say the same call can succeed once that write ends", refusal)
	}
}
