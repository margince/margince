// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A lead file's email and phone cells are judged by the dry run with the parsers
// the lead write runs, so the preview, the commit and a re-import agree.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type leadVersionDTO struct {
	Data []struct {
		Email   string `json:"email"`
		Phone   string `json:"phone"`
		Version int    `json:"version"`
	} `json:"data"`
}

func leadVersions(t *testing.T, e *apptest.AppEnv) leadVersionDTO {
	t.Helper()
	var leads leadVersionDTO
	if status := e.Call(t, http.MethodGet, "/v1/leads?limit=100", nil, nil, &leads); status != http.StatusOK {
		t.Fatalf("GET /v1/leads → %d, want 200", status)
	}
	return leads
}

func TestALeadFileNamesItsBadEmailAndPhoneInThePreviewAndCommitsTheRest(t *testing.T) {
	e := setupImportApp(t)
	file := "Name,Email,Phone\n" +
		"Ada Lovelace,ada@lovelace.example,+44 20 7946 0001\n" +
		"Bad Mail,not-an-email,+44 20 7946 0002\n" +
		"Bad Phone,bad@phone.example,020 7946 0003\n"

	profile, _ := uploadCSV(t, e, "lead", file)
	run, status := createRunWithMapping(t, e, "lead", profile.SourceRef, map[string]string{
		"Name": "full_name", "Email": "email", "Phone": "phone",
	})
	if status != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", status)
	}
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 1 || report.Disposition.Skipped != 2 || len(report.Issues) != 2 {
		t.Fatalf("preview = %+v issues %+v, want 1 create and the 2 bad cells skipped by name", report.Disposition, report.Issues)
	}
	if status := importCall(t, e, run.ID, "approve", nil, &run); status != http.StatusAccepted || run.Status != "complete" {
		t.Fatalf("approve → %d, run %q, want 202 and complete with the good row written", status, run.Status)
	}
	if got := leadCount(t, e); got != 1 {
		t.Fatalf("leads = %d, want the 1 good row", got)
	}
}

func TestReimportingASpacedLeadPhoneChangesNothing(t *testing.T) {
	e := setupImportApp(t)
	file := "Name,Email,Phone\nAda Lovelace,ada@lovelace.example,+84 90 111 0002\n"
	mapping := map[string]string{"Name": "full_name", "Email": "email", "Phone": "phone"}

	first, _ := uploadCSV(t, e, "lead", file)
	run, _ := createRunWithMapping(t, e, "lead", first.SourceRef, mapping)
	if status := importCall(t, e, run.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("first approve → %d, want 202", status)
	}
	before := leadVersions(t, e)

	again, _ := uploadCSV(t, e, "lead", file)
	rerun, _ := createRunWithMapping(t, e, "lead", again.SourceRef, mapping)
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+rerun.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Unchanged != 1 || report.Disposition.Updated != 0 {
		t.Fatalf("re-import preview = %+v, want the one row unchanged", report.Disposition)
	}
	if status := importCall(t, e, rerun.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("second approve → %d, want 202", status)
	}
	after := leadVersions(t, e)
	if len(after.Data) != 1 || after.Data[0].Version != before.Data[0].Version || after.Data[0].Phone != "+84901110002" {
		t.Fatalf("lead after re-import = %+v, was %+v, want the same version and the E.164 phone", after.Data, before.Data)
	}
}

func TestALeadFileNamingAnEmailAnotherLeadHoldsIsRefusedInThePreview(t *testing.T) {
	e := setupImportApp(t)
	createdID(t, e, "/v1/leads", AnyMap{"full_name": "Made In App", "email": "taken@lead.example", "source": "manual"})
	file := "Name,Email\nSomeone Else,taken@lead.example\nFresh Lead,fresh@lead.example\n"

	profile, _ := uploadCSV(t, e, "lead", file)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, map[string]string{"Name": "full_name", "Email": "email"})
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 1 {
		t.Fatalf("preview = %+v, want only the fresh lead created", report.Disposition)
	}
	if status := importCall(t, e, run.ID, "approve", nil, &run); status != http.StatusAccepted || run.Status != "complete" {
		t.Fatalf("approve → %d, run %q (%s), want 202 and complete", status, run.Status, run.Error)
	}
}
