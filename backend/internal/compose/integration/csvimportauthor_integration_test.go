// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An `author` column names who wrote each row in the system the file was
// exported from. The cell is kept as the source's spelling; a cell that is a
// seat's email names that seat too. The author is written when the import
// creates a record and never rewritten by a later file.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const authoredContactsCSV = "Email,Full Name,Author\n" +
	"one@authored.example,One Authored,ADA@example.com\n" +
	"two@authored.example,Two Authored,Anna Müller\n"

// authorOfContact reads a contact's provenance by its address, through the
// owner connection so the assertion does not depend on any read path.
func authorOfContact(t *testing.T, e *apptest.AppEnv, email string) (system, seat, name string) {
	t.Helper()
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT coalesce(c.source_system, ''), coalesce(c.source_author_id::text, ''), coalesce(c.source_author_name, '')
		  FROM contact c JOIN contact_email ce ON ce.contact_id = c.id
		 WHERE ce.email = $1`, email).Scan(&system, &seat, &name); err != nil {
		t.Fatalf("reading the author of %s: %v", email, err)
	}
	return system, seat, name
}

func seatOf(t *testing.T, e *apptest.AppEnv, email string) string {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT id::text FROM app_user WHERE lower(email) = lower($1)`, email).Scan(&id); err != nil {
		t.Fatalf("reading the seat of %s: %v", email, err)
	}
	return id
}

func importAndApprove(t *testing.T, e *apptest.AppEnv, object, csv string, mapping map[string]string) importRunDTO {
	t.Helper()
	profile, status := uploadCSV(t, e, object, csv)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, runStatus := createRunWithMapping(t, e, object, profile.SourceRef, mapping)
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	return run
}

func TestCSVImportNamesTheAuthorOfEachContact(t *testing.T) {
	e := setupImportApp(t)
	mapping := map[string]string{"Email": "email", "Full Name": "full_name", "Author": "author"}
	importAndApprove(t, e, "contact", authoredContactsCSV, mapping)

	system, seat, name := authorOfContact(t, e, "one@authored.example")
	if system != "mirror:csv" {
		t.Errorf("source_system = %q, want mirror:csv — an author needs the source it is a claim about", system)
	}
	if seat != seatOf(t, e, "ada@example.com") || name != "ADA@example.com" {
		t.Errorf("author = (%q, %q), want Ada's seat beside the cell as written", seat, name)
	}
	if _, seat, name := authorOfContact(t, e, "two@authored.example"); seat != "" || name != "Anna Müller" {
		t.Errorf("author = (%q, %q), want the name alone — no seat holds it", seat, name)
	}

	// A later file names somebody else. The record was written by who wrote it
	// when it landed, so nothing moves and the run has nothing to update.
	rewritten := "Email,Full Name,Author\n" +
		"one@authored.example,One Authored,Somebody Else\n" +
		"two@authored.example,Two Authored,Somebody Else\n"
	rerun := importAndApprove(t, e, "contact", rewritten, mapping)
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+rerun.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("re-report → %d, want 200", status)
	}
	if report.Disposition.Created != 0 || report.Disposition.Updated != 0 {
		t.Errorf("re-run = %+v, want nothing created or updated for an author-only change", report.Disposition)
	}
	if _, _, name := authorOfContact(t, e, "two@authored.example"); name != "Anna Müller" {
		t.Errorf("author name after re-import = %q, want it unchanged", name)
	}
}

func TestCSVImportNamesTheAuthorOfEachCompany(t *testing.T) {
	e := setupImportApp(t)
	importAndApprove(t, e, "company", "Name,Author\nAuthored Company GmbH,Anna Müller\n",
		map[string]string{"Name": "display_name", "Author": "author"})

	var system, name string
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT coalesce(source_system, ''), coalesce(source_author_name, '')
		  FROM company WHERE display_name = 'Authored Company GmbH'`).Scan(&system, &name); err != nil {
		t.Fatalf("reading the imported company: %v", err)
	}
	if system != "mirror:csv" || name != "Anna Müller" {
		t.Errorf("company provenance = (%q, %q), want mirror:csv and the author", system, name)
	}
}

// The author is part of the create, so it writes no second audit row that
// would mark the record as touched by a human and hold it back from undo.
func TestAnAuthoredImportStillUndoes(t *testing.T) {
	e := setupImportApp(t)
	run := importAndApprove(t, e, "lead", "Email,Full Name,Author\nlead@authored.example,Lead Authored,Anna Müller\n",
		map[string]string{"Email": "email", "Full Name": "full_name", "Author": "author"})

	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/undo", nil, nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d, want 202", status)
	}
	var report importReportWithUndoDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report after undo → %d, want 200", status)
	}
	if report.Undo == nil || report.Undo.ReversedCount != 1 || len(report.Undo.Kept) != 0 {
		t.Fatalf("undo = %+v, want the authored lead reversed", report.Undo)
	}
}
