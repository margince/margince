// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// contactRefParts is one contacts.csv whose company column names a company_ref.
func contactRefParts() []AnyMap {
	return []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
		"ref": "ref", "email": "email", "name": "full_name", "company": "company_ref",
	}}}
}

// hasIssue reports whether the report names an issue of this kind about this
// cell value.
func hasIssue(report importReportDTO, kind, value string) bool {
	return issueOf(report, kind, value) != nil
}

func issueOf(report importReportDTO, kind, value string) *importIssueDTO {
	for i, issue := range report.Issues {
		if issue.Kind == kind && issue.Value == value {
			return &report.Issues[i]
		}
	}
	return nil
}

// readImportReport reads a run's report and requires a 200.
func readImportReport(t *testing.T, e *apptest.AppEnv, runID string) importReportDTO {
	t.Helper()
	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+runID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	return report
}

// employerOf is the name of the company the contact with this address works at
// today. It reads the contact list as a user does.
func employerOf(t *testing.T, e *apptest.AppEnv, email string) string {
	t.Helper()
	var contacts struct {
		Data []struct {
			PrimaryEmail *string `json:"primary_email"`
			Employer     *struct {
				CompanyName string `json:"company_name"`
			} `json:"employer"`
		} `json:"data"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/contacts?limit=100", nil, nil, &contacts); status != http.StatusOK {
		t.Fatalf("GET /v1/contacts → %d, want 200", status)
	}
	for _, contact := range contacts.Data {
		if contact.PrimaryEmail != nil && *contact.PrimaryEmail == email {
			if contact.Employer == nil {
				return ""
			}
			return contact.Employer.CompanyName
		}
	}
	t.Fatalf("no contact holds %s", email)
	return ""
}

func TestAContactLinksToACompanyInTheSameZip(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"companies.csv": "ref,name\nCO-1,Linked GmbH\n",
		"contacts.csv":  "ref,email,name,company\nP-1,jo@linked.example,Jo,CO-1\n",
	}
	parts := append([]AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}},
	}, contactRefParts()...)
	run := stageBundle(t, e, uploadZip(t, e, "l.zip", zipBytes(t, files)).SourceRef, "l", parts)
	report := readImportReport(t, e, run.ID)
	if report.Links == nil || report.Links.Offered != 1 || len(report.Links.Unresolved) != 0 {
		t.Fatalf("links = %+v, want one link and none unresolved before either record exists", report.Links)
	}
	if report.Disposition.Created != 2 || len(report.Issues) != 0 {
		t.Fatalf("report = %+v, want both rows created and no issue", report)
	}
	approveAndRead(t, e, run.ID)
	if employer := employerOf(t, e, "jo@linked.example"); employer != "Linked GmbH" {
		t.Fatalf("employer = %q", employer)
	}
	if links := readImportReport(t, e, run.ID).Links; links == nil || links.Applied != 1 {
		t.Fatalf("links after the commit = %+v, want the one link applied", links)
	}
}

func TestABrokenLinkSkipsTheRowUnlessTheRunSaysImportAnyway(t *testing.T) {
	for _, policy := range []string{"skip_row", "drop_link"} {
		t.Run(policy, func(t *testing.T) {
			e := setupImportApp(t)
			files := map[string]string{"contacts.csv": "ref,email,name,company\nP-1,kim@" + policy + ".example,Kim,CO-404\n"}
			run := stageRefRun(t, e, files, policy)
			wantKind, wantCreated := "broken_link", 0
			if policy == "drop_link" {
				wantKind, wantCreated = "dropped_link", 1
			}
			report := readImportReport(t, e, run.ID)
			if report.Disposition.Created != wantCreated || !hasIssue(report, wantKind, "CO-404") {
				t.Fatalf("report = %+v, want %d created and a %s issue naming CO-404", report, wantCreated, wantKind)
			}
			if issue := issueOf(report, wantKind, "CO-404"); issue.Column != "company" || issue.Line != 2 || issue.File != "contacts.csv" {
				t.Fatalf("issue = %+v, want the company column on line 2 of contacts.csv", *issue)
			}
			approveAndRead(t, e, run.ID)
			committed := readImportReport(t, e, run.ID)
			if committed.Disposition.Created != wantCreated || !hasIssue(committed, wantKind, "CO-404") {
				t.Fatalf("committed = %+v, want the commit to do what the dry run said", committed)
			}
		})
	}
}

// stageRefRun stages contacts.csv of a ZIP under a broken-link policy.
func stageRefRun(t *testing.T, e *apptest.AppEnv, files map[string]string, policy string) importRunDTO {
	t.Helper()
	var run importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports", AnyMap{
		"connector": "csv", "source_ref": uploadZip(t, e, "b.zip", zipBytes(t, files)).SourceRef,
		"source_name": "b", "on_broken_link": policy, "parts": contactRefParts(),
	}, nil, &run); status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	return run
}

// A colleague's owner-private company is not found by its id or by its name.
// The words match a company that does not exist, or they would tell the
// importer the company is there.
func TestACompanyTheCallerCannotSeeIsNotFoundByIDOrByName(t *testing.T) {
	e := setupImportApp(t)
	// A colleague is invited only once the workspace has described its company.
	e.DescribeCompany(t)
	colleague := createdID(t, e, "/v1/users", AnyMap{
		"email": "colleague@hidden-link.test", "display_name": "Colleague", "role": "rep",
	})
	hidden := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Hidden Works"})
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE company SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden, colleague); err != nil {
		t.Fatalf("making the company private to the colleague: %v", err)
	}

	files := map[string]string{"contacts.csv": "ref,email,name,company\n" +
		"P-1,a@hidden-link.test,A," + hidden + "\n" +
		"P-2,b@hidden-link.test,B,Hidden Works\n" +
		"P-3,c@hidden-link.test,C,CO-404\n"}
	report := readImportReport(t, e, stageRefRun(t, e, files, "skip_row").ID)
	missing := issueOf(report, "broken_link", "CO-404")
	if missing == nil {
		t.Fatalf("report = %+v, want CO-404 reported as a broken link", report)
	}
	for _, value := range []string{hidden, "Hidden Works"} {
		issue := issueOf(report, "broken_link", value)
		if issue == nil {
			t.Fatalf("report = %+v, want %q reported as a broken link", report, value)
		}
		if want := strings.ReplaceAll(missing.Reason, "CO-404", value); issue.Reason != want {
			t.Errorf("reason for %q = %q, want %q: the words for a company that does not exist", value, issue.Reason, want)
		}
	}
	if report.Disposition.Created != 0 {
		t.Fatalf("created = %d, want every row skipped", report.Disposition.Created)
	}
}
