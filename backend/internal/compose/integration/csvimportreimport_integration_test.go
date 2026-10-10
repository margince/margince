// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/database"
)

func contactImportMapping() map[string]string {
	return map[string]string{"Email": "email", "Full Name": "full_name", "Title": "title"}
}

// importContactFile uploads, stages and approves one contact file and returns
// the run's report.
func importContactFile(t *testing.T, e *apptest.AppEnv, file string) (string, importReportDTO) {
	t.Helper()
	profile, status := uploadCSV(t, e, "contact", file)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, status := createRunWithMapping(t, e, "contact", profile.SourceRef, contactImportMapping())
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}
	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("report → %d, want 200", s)
	}
	return run.ID, report
}

func firstContactID(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	var list struct {
		Data []AnyMap `json:"data"`
	}
	if s := e.Call(t, http.MethodGet, "/v1/contacts?limit=100", nil, nil, &list); s != http.StatusOK || len(list.Data) == 0 {
		t.Fatalf("GET /v1/contacts → %d with %d rows, want one", s, len(list.Data))
	}
	id, ok := list.Data[0]["id"].(string)
	if !ok {
		t.Fatalf("contact has no string id: %v", list.Data[0])
	}
	return id
}

func setRunStatus(t *testing.T, e *apptest.AppEnv, runID, status string) {
	t.Helper()
	ctx := e.DealWriterContext(t)
	err := database.WithWorkspaceTx(ctx, e.DB().Pool(), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE import_run SET status = $2 WHERE id = $1`, runID, status)
		return err
	})
	if err != nil {
		t.Fatalf("setting the run's status: %v", err)
	}
}

// The email is one key however it is cased: a corrected file that spells the
// address differently updates the contact the first file made.
func TestAReImportMatchesAnEmailWhateverItsCase(t *testing.T) {
	e := setupImportApp(t)
	importContactFile(t, e, "Email,Full Name,Title\nAda@Lovelace.example,Ada Lovelace,Old title\n")

	_, report := importContactFile(t, e, "Email,Full Name,Title\nada@lovelace.example,Ada Lovelace,New title\n")

	if report.Disposition.Updated != 1 || report.Disposition.Created != 0 || report.Disposition.Skipped != 0 {
		t.Fatalf("disposition = %+v, want one update and nothing created or skipped", report.Disposition)
	}
	if got := contactTitle(t, e, "ada@lovelace.example"); got != "New title" {
		t.Fatalf("title = %q, want New title", got)
	}
}

// A contact archived by hand is not re-created as a twin: the row is skipped
// and says why.
func TestAReImportSkipsARowWhoseContactWasArchived(t *testing.T) {
	e := setupImportApp(t)
	file := "Email,Full Name,Title\nada@lovelace.example,Ada Lovelace,Analyst\n"
	importContactFile(t, e, file)
	if s := e.Call(t, http.MethodDelete, "/v1/contacts/"+firstContactID(t, e), nil, nil, nil); s != http.StatusOK && s != http.StatusNoContent {
		t.Fatalf("archive → %d, want 200 or 204", s)
	}

	_, report := importContactFile(t, e, file)

	if report.Disposition.Skipped != 1 || report.Disposition.Created != 0 {
		t.Fatalf("disposition = %+v, want the row skipped and nothing created", report.Disposition)
	}
	if len(report.Issues) != 1 || !strings.Contains(report.Issues[0].Reason, "archived") {
		t.Fatalf("issues = %+v, want one that says the record is archived", report.Issues)
	}
	if got := importedContactCount(t, e); got != 0 {
		t.Fatalf("live contacts = %d, want 0: no twin", got)
	}
}

// A key whose run is only part-way through an undo is still bound: re-importing
// updates the record and leaves the paused undo's rows in place.
func TestAReImportDoesNotTakeOverAKeyWhileItsUndoIsRunning(t *testing.T) {
	e := setupImportApp(t)
	runID, _ := importContactFile(t, e, "Email,Full Name,Title\nada@lovelace.example,Ada Lovelace,Old title\n")
	setRunStatus(t, e, runID, "undoing")

	_, report := importContactFile(t, e, "Email,Full Name,Title\nada@lovelace.example,Ada Lovelace,New title\n")

	if report.Disposition.Updated != 1 || report.Disposition.Created != 0 {
		t.Fatalf("disposition = %+v, want the existing contact updated", report.Disposition)
	}
	if got := importedContactCount(t, e); got != 1 {
		t.Fatalf("live contacts = %d, want 1", got)
	}
}
