// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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
	if status := testCall(t, e, "/v1/imports", AnyMap{
		"connector": "csv", "source_ref": uploadZip(t, e, "b.zip", zipBytes(t, files)).SourceRef,
		"source_name": "b", "on_broken_link": policy, "parts": contactRefParts(),
	}, &run); status != http.StatusAccepted {
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

// A company row naming its record by id also carries a ref. A contact of the
// same upload linking to that ref is created and linked by the commit, as the
// test run said it would be.
func TestAContactLinksToACompanyTheZipNamesByID(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	files := map[string]string{
		"companies.csv": "ref,id,name\nCO-7," + named + ",Named Works AG\n",
		"contacts.csv":  "ref,email,name,company\nP-1,ida@named.example,Ida,CO-7\n",
	}
	parts := append([]AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "id": "id", "name": "display_name"}},
	}, contactRefParts()...)
	run := stageBundle(t, e, uploadZip(t, e, "n.zip", zipBytes(t, files)).SourceRef, "n", parts)
	predicted := readImportReport(t, e, run.ID)
	if predicted.Links == nil || predicted.Links.Offered != 1 || len(predicted.Links.Unresolved) != 0 || hasIssue(predicted, "broken_link", "CO-7") {
		t.Fatalf("dry run = %+v, want the link predicted and no broken link", predicted)
	}
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if committed.Disposition.Created != 1 || committed.Disposition.Updated != 1 || hasIssue(committed, "broken_link", "CO-7") {
		t.Fatalf("committed = %+v, want the contact created, the company updated and no broken link", committed)
	}
	if employer := employerOf(t, e, "ida@named.example"); employer != "Named Works AG" {
		t.Fatalf("employer = %q, want the company the row named by id", employer)
	}
}

// A new employer name creates its company in CSV and ZIP imports.
// A missing company reference still follows the broken-link policy.
func TestACompanyNameThatFindsNothingFollowsTheZipsPolicy(t *testing.T) {
	nameParts := []AnyMap{{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
		"ref": "ref", "email": "email", "name": "full_name", "company": "company_name",
	}}}
	for _, policy := range []string{"skip_row", "drop_link"} {
		t.Run(policy, func(t *testing.T) {
			e := setupImportApp(t)
			files := map[string]string{"contacts.csv": "ref,email,name,company\nP-1,lu@" + policy + ".example,Lu,Nowhere Ltd\n"}
			var run importRunDTO
			if status := testCall(t, e, "/v1/imports", AnyMap{
				"connector": "csv", "source_ref": uploadZip(t, e, "c.zip", zipBytes(t, files)).SourceRef,
				"source_name": "c", "on_broken_link": policy, "parts": nameParts,
			}, &run); status != http.StatusAccepted {
				t.Fatalf("stage → %d, want 202", status)
			}

			report := readImportReport(t, e, run.ID)
			if report.Disposition.Created != 1 || report.Links == nil || len(report.Links.Unresolved) != 0 {
				t.Fatalf("preview=%+v, want one contact and its employer", report)
			}
			approveAndRead(t, e, run.ID)
			if committed := readImportReport(t, e, run.ID); committed.Disposition.Created != 1 || employerOf(t, e, "lu@"+policy+".example") != "Nowhere Ltd" {
				t.Fatalf("commit=%+v, want one contact at Nowhere Ltd", committed)
			}
		})
	}
	t.Run("single csv", func(t *testing.T) {
		e := setupImportApp(t)
		profile, status := uploadCSV(t, e, "contact", "Email,Name,Company\nlu@single.example,Lu,Nowhere Ltd\n")
		if status != http.StatusOK {
			t.Fatalf("upload → %d, want 200", status)
		}
		run, status := createRunWithMapping(t, e, "contact", profile.SourceRef,
			map[string]string{"Email": "email", "Name": "full_name", "Company": "company_name"})
		if status != http.StatusAccepted {
			t.Fatalf("stage → %d, want 202", status)
		}
		report := readImportReport(t, e, run.ID)
		if report.Disposition.Created != 1 || len(report.Issues) != 1 || report.Issues[0].ReasonCode != "employer_will_create" || report.Links == nil || len(report.Links.Unresolved) != 0 {
			t.Fatalf("report = %+v, want the row and employer created with a preview notice", report)
		}
	})
}

// A name only a company of the same ZIP wears resolves in the test run as at
// commit. The default skip_row then keeps the contact.
func TestAContactNamesACompanyOfTheSameZip(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"companies.csv": "ref,name\nCO-1,Zipped Works\n",
		"contacts.csv":  "ref,email,name,company\nP-1,mo@zipped.example,Mo,zipped  works\n",
	}
	parts := []AnyMap{
		{"file": "companies.csv", "object": "company", "mapping": AnyMap{"ref": "ref", "name": "display_name"}},
		{"file": "contacts.csv", "object": "contact", "mapping": AnyMap{
			"ref": "ref", "email": "email", "name": "full_name", "company": "company_name",
		}},
	}
	run := stageBundle(t, e, uploadZip(t, e, "z.zip", zipBytes(t, files)).SourceRef, "z", parts)
	predicted := readImportReport(t, e, run.ID)
	if predicted.Disposition.Created != 2 || hasIssue(predicted, "broken_link", "zipped  works") ||
		predicted.Links == nil || predicted.Links.Offered != 1 || len(predicted.Links.Unresolved) != 0 {
		t.Fatalf("dry run = %+v, want both rows created and the link predicted", predicted)
	}
	approveAndRead(t, e, run.ID)
	if committed := readImportReport(t, e, run.ID); committed.Disposition.Created != 2 {
		t.Fatalf("committed = %+v, want both rows created", committed)
	}
	if employer := employerOf(t, e, "mo@zipped.example"); employer != "Zipped Works" {
		t.Fatalf("employer = %q, want the company of the same ZIP", employer)
	}
}

// A lead's company_name is text it stores, not a link, so a ZIP run lands the
// lead whatever the name finds.
func TestALeadsCompanyNameNeverSkipsTheLead(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{"leads.csv": "ref,email,name,company\nL-1,lea@lead.example,Lea,Nowhere Ltd\n"}
	parts := []AnyMap{{"file": "leads.csv", "object": "lead", "mapping": AnyMap{
		"ref": "ref", "email": "email", "name": "full_name", "company": "company_name",
	}}}
	run := stageBundle(t, e, uploadZip(t, e, "l.zip", zipBytes(t, files)).SourceRef, "l", parts)
	if predicted := readImportReport(t, e, run.ID); predicted.Disposition.Created != 1 || len(predicted.Issues) != 0 {
		t.Fatalf("dry run = %+v, want the lead created and no issue", predicted)
	}
	approveAndRead(t, e, run.ID)
	if committed := readImportReport(t, e, run.ID); committed.Disposition.Created != 1 || len(committed.Issues) != 0 {
		t.Fatalf("committed = %+v, want the lead created and no issue", committed)
	}
}

// namedCompanyZip stages a ZIP whose company row names an existing company by
// id under ref CO-7, and whose contact links to CO-7.
func namedCompanyZip(t *testing.T, e *apptest.AppEnv, companyCSV string) importRunDTO {
	t.Helper()
	files := map[string]string{
		"companies.csv": companyCSV,
		"contacts.csv":  "ref,email,name,company\nP-1,ida@named.example,Ida,CO-7\n",
	}
	parts := append([]AnyMap{{"file": "companies.csv", "object": "company", "mapping": AnyMap{
		"ref": "ref", "id": "id", "name": "display_name", "domain": "domain",
	}}}, contactRefParts()...)
	return stageBundle(t, e, uploadZip(t, e, "n.zip", zipBytes(t, files)).SourceRef, "n", parts)
}

// failAfterTheCompanyRow leaves the run as an attempt that landed the company
// row and died: failed past it, with the bound ref that row wrote.
func failAfterTheCompanyRow(t *testing.T, e *apptest.AppEnv, runID, named string) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	user := ids.NewV7()
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{migration.ImportRunObject: {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	company, err := ids.Parse(named)
	if err != nil {
		t.Fatal(err)
	}
	system := provenance.ReservedSourceSystemPrefix + migration.ConnectorCSV + ":n"
	if err := database.WithWorkspaceTx(ctx, e.DB().Pool(), func(tx pgx.Tx) error {
		run, err := ids.Parse(runID)
		if err != nil {
			return err
		}
		if err := migration.NewRunStore(e.DB()).RecordIdentityTx(ctx, tx, migration.RunID(run),
			system, migration.ObjectCompany, "CO-7", company, migration.OriginBound); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE import_run SET status = 'failed', checkpoint = 1, approved_at = now() WHERE id = $1`, runID)
		return err
	}); err != nil {
		t.Fatalf("failing the run after its company row: %v", err)
	}
}

// A resumed commit has passed the company row, yet the contact still finds the
// company that row named by id, as the test run said.
func TestAResumedCommitStillFindsACompanyNamedByID(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	run := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Named Works,\n")
	failAfterTheCompanyRow(t, e, run.ID, named)
	approveAndRead(t, e, run.ID)
	if employer := employerOf(t, e, "ida@named.example"); employer != "Named Works" {
		t.Fatalf("employer = %q, want the company the passed row named", employer)
	}
}

// A resumed commit checks the company a passed row named by id under the
// principal resuming it. One the importer can no longer see is a broken link.
func TestAResumedCommitDoesNotLinkACompanyTheCallerCanNoLongerSee(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	colleague := createdID(t, e, "/v1/users", AnyMap{
		"email": "colleague@named-hidden.test", "display_name": "Colleague", "role": "rep",
	})
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	run := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Named Works,\n")
	failAfterTheCompanyRow(t, e, run.ID, named)
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE company SET visibility = 'owner', owner_id = $2 WHERE id = $1`, named, colleague); err != nil {
		t.Fatalf("making the named company private to the colleague: %v", err)
	}
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if !hasIssue(committed, "broken_link", "CO-7") || committed.Disposition.Created != 0 {
		t.Fatalf("committed = %+v, want CO-7 a broken link and the contact skipped", committed)
	}
}

// A company row naming its record by id still binds its ref when the update is
// refused for a domain another company holds. The contact links as predicted.
func TestARefusedUpdateStillBindsTheCompanyItNamed(t *testing.T) {
	e := setupImportApp(t)
	createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Holder", "domains": []AnyMap{
		{"domain": "held.example", "is_primary": true},
	}})
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	run := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Named Works,held.example\n")
	if predicted := readImportReport(t, e, run.ID); hasIssue(predicted, "broken_link", "CO-7") {
		t.Fatalf("dry run = %+v, want no broken link", predicted)
	}
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if committed.Disposition.Created != 1 || hasIssue(committed, "broken_link", "CO-7") {
		t.Fatalf("committed = %+v, want the contact created and no broken link", committed)
	}
	if employer := employerOf(t, e, "ida@named.example"); employer != "Named Works" {
		t.Fatalf("employer = %q, want the company the row named", employer)
	}
}

// A company row naming its record by id binds its ref for good. A later import
// under the same source name links a contact to it by that ref.
func TestALaterImportFindsTheRefOfAnIdNamedCompany(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	approveAndRead(t, e, namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Named Works,\n").ID)

	later := map[string]string{"contacts.csv": "ref,email,name,company\nP-2,ivo@named.example,Ivo,CO-7\n"}
	run := stageBundle(t, e, uploadZip(t, e, "later.zip", zipBytes(t, later)).SourceRef, "n", contactRefParts())
	if predicted := readImportReport(t, e, run.ID); hasIssue(predicted, "broken_link", "CO-7") {
		t.Fatalf("dry run = %+v, want CO-7 found through the earlier import", predicted)
	}
	approveAndRead(t, e, run.ID)
	if committed := readImportReport(t, e, run.ID); committed.Disposition.Created != 1 || hasIssue(committed, "broken_link", "CO-7") {
		t.Fatalf("committed = %+v, want the contact created and no broken link", committed)
	}
	if employer := employerOf(t, e, "ivo@named.example"); employer != "Named Works" {
		t.Fatalf("employer = %q, want the company the earlier file named by id", employer)
	}
}

// Undo archives what a run created. The company a row named by id existed
// before the run, so it stays live while the contact the run created goes.
func TestUndoLeavesTheCompanyAFileNamedByID(t *testing.T) {
	e := setupImportApp(t)
	named := createdID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Named Works"})
	run := namedCompanyZip(t, e, "ref,id,name,domain\nCO-7,"+named+",Named Works,\n")
	approveAndRead(t, e, run.ID)

	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d, want 202", status)
	}
	var report importReportWithUndoDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report after undo → %d, want 200", status)
	}
	if report.Undo == nil || report.Undo.ReversedCount != 1 {
		t.Fatalf("undo = %+v, want only the created contact reversed", report.Undo)
	}
	var live bool
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT archived_at IS NULL FROM company WHERE id = $1`, named).Scan(&live); err != nil || !live {
		t.Fatalf("the named company live = %v, %v; want it untouched by the undo", live, err)
	}
}
