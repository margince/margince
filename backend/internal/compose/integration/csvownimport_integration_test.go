// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

// seatID is the id of the seat signed in with email.
func seatID(t *testing.T, e *apptest.AppEnv, email string) string {
	t.Helper()
	var id string
	if err := e.Owner.QueryRow(t.Context(), `SELECT id::text FROM app_user WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// stageImport tests a one-file mapping and answers the status and the body.
func stageImport(t *testing.T, e *apptest.AppEnv, object, sourceRef string, mapping map[string]string) (int, AnyMap) {
	t.Helper()
	var out AnyMap
	status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "object": object, "source_ref": sourceRef, "mapping": mapping,
	}, nil, &out)
	return status, out
}

// roleSeat signs in a new member of role, as repSeat does for a rep.
func roleSeat(t *testing.T, e *apptest.AppEnv, email, role string) *apptest.AppEnv {
	t.Helper()
	const password = "own-import-password"
	seat := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Mia Manager", "role": role})
	apptest.SetPasswordByLink(t, e, seat, password)
	return e.AsSecondSeat(t, email, password)
}

// Management sees every lead but imports only its own. A row that names a
// colleague's lead by id is skipped: the lead keeps its name and does
// not become theirs.
func TestAnOwnImportLeavesAColleaguesRecordAlone(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	lead := createdRecord(t, e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Ada Kept", "email": "kept@own-import.example"})
	boss := roleSeat(t, e, "boss@own-import.test", "management")

	profile, _ := importjobtest.UploadCSV(t, boss, "lead", "id,Name,Email\n"+lead+",Taken Over,kept@own-import.example\n")
	status, run := stageImport(t, boss, "lead", profile.SourceRef, map[string]string{"id": "id", "Name": "full_name", "Email": "email"})
	if status != http.StatusAccepted {
		t.Fatalf("the management import → %d %v, want 202", status, run)
	}
	runID, _ := run["id"].(string)
	importjobtest.Drain(t, boss)
	approveAndRead(t, boss, runID)

	var name, owner string
	if err := e.Owner.QueryRow(t.Context(), `SELECT full_name, coalesce(owner_id::text, '') FROM lead WHERE id = $1`, lead).Scan(&name, &owner); err != nil {
		t.Fatalf("reading the lead: %v", err)
	}
	if name != "Ada Kept" || owner != "" {
		t.Fatalf("the colleague's lead reads %q owned by %q, want %q with no owner", name, owner, "Ada Kept")
	}
}

// A rep imports their own leads. Every row they import is theirs, even where
// the file names a colleague as its owner.
func TestARepImportsLeadsTheyOwn(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	rep := repSeat(t, e, "rep@own-import.test")
	repID := seatID(t, e, "rep@own-import.test")

	profile, _ := importjobtest.UploadCSV(t, rep, "lead", "Name,Email,Owner\nLea Own,lea@own-import.example,"+importAdminEmail+"\n")
	status, run := stageImport(t, rep, "lead", profile.SourceRef, map[string]string{"Name": "full_name", "Email": "email", "Owner": "owner_email"})
	if status != http.StatusAccepted {
		t.Fatalf("the rep's lead import → %d %v, want 202", status, run)
	}
	runID, _ := run["id"].(string)
	importjobtest.Drain(t, rep)
	approveAndRead(t, rep, runID)

	var owner string
	if err := e.Owner.QueryRow(t.Context(), `SELECT coalesce(owner_id::text, '') FROM lead WHERE full_name = 'Lea Own'`).Scan(&owner); err != nil {
		t.Fatalf("reading the imported lead: %v", err)
	}
	if owner != repID {
		t.Fatalf("the imported lead is owned by %q, want the rep %q", owner, repID)
	}
}

// Companies are a workspace import: a rep's run that names them is refused
// before anything is tested.
func TestARepCannotImportCompanies(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	rep := repSeat(t, e, "rep@no-companies.test")

	profile, _ := importjobtest.UploadCSV(t, rep, "company", "Name\nAcme Rep GmbH\n")
	if status, body := stageImport(t, rep, "company", profile.SourceRef, map[string]string{"Name": "display_name"}); status != http.StatusUnprocessableEntity {
		t.Fatalf("the rep's company import → %d %v, want 422", status, body)
	}
}

// A rep sees the runs they started and no one else's.
func TestARepDoesNotSeeAnAdminsImportRun(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	rep := repSeat(t, e, "rep@own-runs.test")

	profile, _ := importjobtest.UploadCSV(t, e, "company", "Name\nAdmin Only GmbH\n")
	adminRun, status := importjobtest.CreateRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name"})
	if status != http.StatusAccepted {
		t.Fatalf("the admin's import → %d, want 202", status)
	}

	if status := rep.Call(t, http.MethodGet, "/v1/imports/"+adminRun.ID, nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("the rep reading the admin's run → %d, want 404", status)
	}
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := rep.Call(t, http.MethodGet, "/v1/imports", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("the rep listing runs → %d, want 200", status)
	}
	for _, run := range page.Data {
		if run.ID == adminRun.ID {
			t.Fatal("the rep's run list shows the admin's run")
		}
	}
}
