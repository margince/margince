// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The dry run judges a lead file's cells with the lead write's own parsers.
// So the preview, the commit and a re-import agree.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/migration"
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

func leadByEmail(t *testing.T, e *apptest.AppEnv, email string) (phone string, version int) {
	t.Helper()
	for _, lead := range leadVersions(t, e).Data {
		if lead.Email == email {
			return lead.Phone, lead.Version
		}
	}
	t.Fatalf("no lead holds %q", email)
	return "", 0
}

func leadReport(t *testing.T, e *apptest.AppEnv, runID string) importReportDTO {
	t.Helper()
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+runID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	return report
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
	report := leadReport(t, e, run.ID)
	if report.Disposition.Created != 1 || report.Disposition.Skipped != 2 || len(report.Issues) != 2 {
		t.Fatalf("preview = %+v issues %+v, want 1 create and the 2 bad cells skipped by name", report.Disposition, report.Issues)
	}
	columns := []string{report.Issues[0].Column, report.Issues[1].Column}
	slices.Sort(columns)
	if !slices.Equal(columns, []string{"Email", "Phone"}) {
		t.Fatalf("issue columns = %v, want the Email and the Phone cell named", columns)
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
	_, versionBefore := leadByEmail(t, e, "ada@lovelace.example")

	again, _ := uploadCSV(t, e, "lead", file)
	rerun, _ := createRunWithMapping(t, e, "lead", again.SourceRef, mapping)
	if report := leadReport(t, e, rerun.ID); report.Disposition.Unchanged != 1 || report.Disposition.Updated != 0 {
		t.Fatalf("re-import preview = %+v, want the one row unchanged", report.Disposition)
	}
	if status := importCall(t, e, rerun.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("second approve → %d, want 202", status)
	}
	phone, version := leadByEmail(t, e, "ada@lovelace.example")
	if version != versionBefore || phone != "+84901110002" {
		t.Fatalf("lead after re-import is at version %d with phone %q, want version %d and the E.164 phone", version, phone, versionBefore)
	}
}

func TestALeadFileNamingAnEmailAnotherLeadHoldsIsSkippedInThePreview(t *testing.T) {
	e := setupImportApp(t)
	createdID(t, e, "/v1/leads", AnyMap{"full_name": "Made In App", "email": "taken@lead.example", "source": "manual"})
	_, versionBefore := leadByEmail(t, e, "taken@lead.example")
	file := "Name,Email\nSomeone Else,taken@lead.example\nFresh Lead,fresh@lead.example\n"

	profile, _ := uploadCSV(t, e, "lead", file)
	run, _ := createRunWithMapping(t, e, "lead", profile.SourceRef, map[string]string{"Name": "full_name", "Email": "email"})
	report := leadReport(t, e, run.ID)
	if report.Disposition.Created != 1 || report.Disposition.Skipped != 1 {
		t.Fatalf("preview = %+v, want the fresh lead created and the held address skipped", report.Disposition)
	}
	if want := migration.Why(migration.ReasonNaturalKeyTaken).English(); len(report.Issues) != 1 || report.Issues[0].Reason != want {
		t.Fatalf("issues = %+v, want the held address skipped with %q", report.Issues, want)
	}
	if status := importCall(t, e, run.ID, "approve", nil, &run); status != http.StatusAccepted || run.Status != "complete" {
		t.Fatalf("approve → %d, run %q (%s), want 202 and complete", status, run.Status, run.Error)
	}
	if got := leadCount(t, e); got != 2 {
		t.Fatalf("leads = %d, want the app-made lead and the fresh one", got)
	}
	if _, version := leadByEmail(t, e, "taken@lead.example"); version != versionBefore {
		t.Fatalf("the app-made lead moved from version %d to %d; the import must not touch it", versionBefore, version)
	}
}

func TestACorrectedLeadFileMovingAnEmailOntoAnotherLeadSkipsTheRow(t *testing.T) {
	e := setupImportApp(t)
	mapping := map[string]string{"Ref": "ref", "Name": "full_name", "Email": "email"}
	runWith := func(file string) importRunDTO {
		profile, _ := uploadCSV(t, e, "lead", file)
		var run importRunDTO
		status := testCall(t, e, "/v1/imports", AnyMap{
			"connector": "csv", "object": "lead", "source_ref": profile.SourceRef,
			"mapping": mapping, "source_key": "Ref",
		}, &run)
		if status != http.StatusAccepted {
			t.Fatalf("create run → %d, want 202", status)
		}
		return run
	}
	first := runWith("Ref,Name,Email\nr1,Ada,a@lead.example\n")
	if status := importCall(t, e, first.ID, "approve", nil, nil); status != http.StatusAccepted {
		t.Fatalf("first approve → %d, want 202", status)
	}
	createdID(t, e, "/v1/leads", AnyMap{"full_name": "Made In App", "email": "b@lead.example", "source": "manual"})
	_, versionBefore := leadByEmail(t, e, "a@lead.example")

	moved := runWith("Ref,Name,Email\nr1,Ada,b@lead.example\n")
	if status := importCall(t, e, moved.ID, "approve", nil, &moved); status != http.StatusAccepted || moved.Status != "complete" {
		t.Fatalf("approve → %d, run %q (%s), want 202 and complete", status, moved.Status, moved.Error)
	}
	if report := leadReport(t, e, moved.ID); report.Disposition.Skipped != 1 || report.Disposition.Updated != 0 {
		t.Fatalf("report = %+v, want the moved row skipped", report.Disposition)
	}
	if _, version := leadByEmail(t, e, "a@lead.example"); version != versionBefore {
		t.Fatalf("the lead moved from version %d to %d, want it untouched", versionBefore, version)
	}
}
