// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A projects file lands through the same run as every other type. A test run
// reports each row, an approval commits it, and an undo reverses it. These cases
// hold that the test run and the commit answer each project row alike. They also
// hold that an export of projects is a file the import reads back.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// stageFile uploads one CSV as an object and maps every column to the target of
// its own name. It answers the test run's report with the run's id.
func stageFile(t *testing.T, e *apptest.AppEnv, object, body string) (string, importReportDTO) {
	t.Helper()
	profile, status := importjobtest.UploadCSV(t, e, object, body)
	if status != http.StatusOK {
		t.Fatalf("uploading the %s file → %d", object, status)
	}
	header, _, _ := strings.Cut(body, "\n")
	mapping := map[string]string{}
	for column := range strings.SplitSeq(header, ",") {
		mapping[column] = column
	}
	run, status := importjobtest.CreateRunWithMapping(t, e, object, profile.SourceRef, mapping)
	if status != http.StatusAccepted {
		t.Fatalf("test run of the %s file → %d", object, status)
	}
	return run.ID, readImportReport(t, e, run.ID)
}

// commitFile approves a staged run and answers the commit's report.
func commitFile(t *testing.T, e *apptest.AppEnv, runID string) importReportDTO {
	t.Helper()
	approveAndRead(t, e, runID)
	return readImportReport(t, e, runID)
}

// wantSameAnswers fails unless the test run and the commit report the same
// disposition and the same issue, with the same code, on every row.
func wantSameAnswers(t *testing.T, preview, committed importReportDTO) {
	t.Helper()
	counts := func(d importDispositionDTO) [4]int { return [4]int{d.Created, d.Updated, d.Unchanged, d.Skipped} }
	if counts(preview.Disposition) != counts(committed.Disposition) {
		t.Errorf("test run %v, commit %v, want the same created, updated, unchanged and skipped", counts(preview.Disposition), counts(committed.Disposition))
	}
	answers := func(report importReportDTO) []string {
		var out []string
		for _, issue := range report.Issues {
			out = append(out, strings.Join([]string{issue.Kind, issue.ReasonCode, issue.Column, issue.Value, issue.Reason}, "|"))
		}
		return out
	}
	if got, want := strings.Join(answers(committed), "\n"), strings.Join(answers(preview), "\n"); got != want {
		t.Errorf("commit issues:\n%s\ntest run issues:\n%s\nwant the same on every row", got, want)
	}
}

// importedProjects reads the live projects the importer left, by name.
func importedProjects(t *testing.T, e *apptest.AppEnv) map[string]importedProjectRow {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(), `
		SELECT p.name, p.id::text, p.phase, p.company_id::text, p.description, p.started_at::text,
		       p.target_end_date::text, p.source_system, p.source_id,
		       (SELECT d.id::text FROM deal d WHERE d.project_id = p.id LIMIT 1)
		  FROM project p WHERE p.archived_at IS NULL`)
	if err != nil {
		t.Fatalf("reading the projects: %v", err)
	}
	defer rows.Close()
	out := map[string]importedProjectRow{}
	for rows.Next() {
		var p importedProjectRow
		if err := rows.Scan(&p.Name, &p.ID, &p.Phase, &p.CompanyID, &p.Description, &p.StartedAt, &p.TargetEnd, &p.SourceSystem, &p.SourceID, &p.DealID); err != nil {
			t.Fatalf("reading a project: %v", err)
		}
		out[p.Name] = p
	}
	return out
}

type importedProjectRow struct {
	Name, ID, Phase, CompanyID                                string
	Description, StartedAt, TargetEnd, SourceSystem, SourceID *string
	DealID                                                    *string
}

// projectWorld is a company, a second company and one open deal of each.
type projectWorld struct{ acme, other, acmeDeal, otherDeal string }

func newProjectWorld(t *testing.T, e *apptest.AppEnv) projectWorld {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	w := projectWorld{
		acme:  createdID(t, e, "/v1/companies", AnyMap{"display_name": "Acme Projects GmbH", "source": "manual"}),
		other: createdID(t, e, "/v1/companies", AnyMap{"display_name": "Other Projects AG", "source": "manual"}),
	}
	w.acmeDeal = createdID(t, e, "/v1/deals", AnyMap{"name": "Acme rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "company_id": w.acme, "source": "manual"})
	w.otherDeal = createdID(t, e, "/v1/deals", AnyMap{"name": "Other rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "company_id": w.other, "source": "manual"})
	return w
}

// startDay and endDay are a project's dates, held to the clock the suite pins.
func projectDays(t *testing.T) (startDay, endDay string) {
	t.Helper()
	now := clocktest.Now(t)
	return now.AddDate(0, -2, 0).Format(time.DateOnly), now.AddDate(0, 4, 0).Format(time.DateOnly)
}

const importProjectHeader = "ref,name,company_ref,deal_ref,phase,started_at,target_end_date,description"

func TestAProjectFileCreatesProjectsOnTheirCompaniesAndDeals(t *testing.T) {
	startDay, endDay := projectDays(t)
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	runID, preview := stageFile(t, e, "project", importProjectHeader+"\n"+
		"P-1,Plant rollout,Acme Projects GmbH,"+w.acmeDeal+",pursuing,"+startDay+","+endDay+",\"First, then second\"\n"+
		"P-2,Second line,"+w.other+",,,,,\n")
	if preview.Disposition.Created != 2 || preview.Disposition.Skipped != 0 {
		t.Fatalf("test run = %+v issues %+v, want both projects created", preview.Disposition, preview.Issues)
	}
	committed := commitFile(t, e, runID)
	wantSameAnswers(t, preview, committed)
	projects := importedProjects(t, e)
	rollout, second := projects["Plant rollout"], projects["Second line"]
	if rollout.CompanyID != w.acme || rollout.Phase != "pursuing" || rollout.DealID == nil || *rollout.DealID != w.acmeDeal ||
		rollout.Description == nil || *rollout.Description != "First, then second" ||
		rollout.StartedAt == nil || *rollout.StartedAt != startDay || rollout.TargetEnd == nil || *rollout.TargetEnd != endDay || rollout.SourceSystem == nil || rollout.SourceID != nil {
		t.Fatalf("project = %+v, want it on Acme, pursuing, started from its deal, with its dates and text", rollout)
	}
	if second.CompanyID != w.other || second.Phase != "initiative" || second.DealID != nil {
		t.Fatalf("project = %+v, want it on the other company, in the birth phase, with no deal", second)
	}
}

func TestTheDryRunAndTheCommitAnswerEveryProjectRowAlike(t *testing.T) {
	startDay, endDay := projectDays(t)
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	runID, preview := stageFile(t, e, "project", importProjectHeader+"\n"+
		"P-1,Fine,"+w.acme+","+w.acmeDeal+",,"+startDay+","+endDay+",\n"+
		"P-2,,"+w.acme+",,,,,\n"+
		"P-3,No company,,,,,,\n"+
		"P-4,Unknown company,Nobody Inc,,,,,\n"+
		"P-5,Born closed,"+w.acme+",,closed,,,\n"+
		"P-6,Odd phase,"+w.acme+",,someday,,,\n"+
		"P-7,Bad date,"+w.acme+",,,not-a-date,,\n"+
		"P-8,Dates swapped,"+w.acme+",,,"+endDay+","+startDay+",\n"+
		"P-9,Deal of another company,"+w.acme+","+w.otherDeal+",,,,\n"+
		"P-10,Same deal twice,"+w.acme+","+w.acmeDeal+",,,,\n")
	if preview.Disposition.Created != 1 || preview.Disposition.Skipped != 9 {
		t.Fatalf("test run = %+v issues %+v, want one project created and nine skipped", preview.Disposition, preview.Issues)
	}
	committed := commitFile(t, e, runID)
	wantSameAnswers(t, preview, committed)
	if projects := importedProjects(t, e); len(projects) != 1 || projects["Fine"].ID == "" {
		t.Fatalf("projects = %v, want only the one row the test run created", projects)
	}
	codes := map[string]bool{}
	for _, issue := range committed.Issues {
		codes[issue.ReasonCode] = true
	}
	for _, want := range []string{"project_unnamed", "project_company_missing", "link_not_found", "project_phase_closed", "project_phase_unknown", "not_a_date", "project_dates_inverted", "project_deal_refused"} {
		if !codes[want] {
			t.Errorf("issue codes = %v, want %s among them", codes, want)
		}
	}
}

func TestAnExportedProjectFileUploadsAsSameAndAnIDRowUpdatesTheProject(t *testing.T) {
	startDay, endDay := projectDays(t)
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	runID, _ := stageFile(t, e, "project", importProjectHeader+"\n"+
		"P-1,Plant rollout,"+w.acme+","+w.acmeDeal+",delivering,"+startDay+","+endDay+",Two lines\n"+
		"P-2,Second line,"+w.other+",,,,,\n")
	commitFile(t, e, runID)

	file, _ := exportAll(t, e, "project")
	report, header, profile := roundTrip(t, e, "project", file)
	wantEverythingSame(t, "project", report, header, profile, 2)

	id := importedProjects(t, e)["Plant rollout"].ID
	corrected, _ := stageFile(t, e, "project", "id,name,description\n"+id+",Plant rollout 2027,Three lines\n")
	preview := readImportReport(t, e, corrected)
	if preview.Disposition.Updated != 1 || preview.Disposition.Created != 0 {
		t.Fatalf("test run = %+v issues %+v, want the named project updated, not a second one created", preview.Disposition, preview.Issues)
	}
	wantSameAnswers(t, preview, commitFile(t, e, corrected))
	projects := importedProjects(t, e)
	if len(projects) != 2 || projects["Plant rollout 2027"].ID != id || projects["Plant rollout 2027"].Phase != "delivering" {
		t.Fatalf("projects = %v, want the named project renamed in place and its phase kept", projects)
	}
}

func TestACorrectedProjectFileCannotMoveAProjectToAnotherCompany(t *testing.T) {
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	runID, _ := stageFile(t, e, "project", importProjectHeader+"\nP-1,Stays put,"+w.acme+",,,,,\n")
	commitFile(t, e, runID)
	id := importedProjects(t, e)["Stays put"].ID
	moved, preview := stageFile(t, e, "project", "id,company_ref\n"+id+","+w.other+"\n")
	if preview.Disposition.Skipped != 1 || len(preview.Issues) != 1 || preview.Issues[0].ReasonCode != "project_company_fixed" {
		t.Fatalf("test run = %+v issues %+v, want the row refused as project_company_fixed", preview.Disposition, preview.Issues)
	}
	wantSameAnswers(t, preview, commitFile(t, e, moved))
	if got := importedProjects(t, e)["Stays put"].CompanyID; got != w.acme {
		t.Fatalf("company = %s, want the project kept on its own", got)
	}
}

func TestUndoArchivesImportedProjectsAndKeepsOneAColleagueEdited(t *testing.T) {
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	runID, _ := stageFile(t, e, "project", importProjectHeader+"\nP-1,Edited by a rep,"+w.acme+",,,,,\nP-2,Left alone,"+w.acme+",,,,,\n")
	commitFile(t, e, runID)
	edited := importedProjects(t, e)["Edited by a rep"].ID
	if status := e.Call(t, http.MethodPatch, "/v1/projects/"+edited, AnyMap{"description": "Called them"}, nil, nil); status != http.StatusOK {
		t.Fatalf("editing the project → %d", status)
	}
	if status := importCall(t, e, runID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	var report importReportWithUndoDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+runID+"/report", nil, nil, &report)
	if report.Undo == nil || report.Undo.ReversedCount != 1 || len(report.Undo.Kept) != 1 || report.Undo.Kept[0].ID != edited {
		t.Fatalf("undo = %+v, want one project reversed and the edited one kept", report.Undo)
	}
	if left := importedProjects(t, e); len(left) != 1 || left["Edited by a rep"].ID != edited {
		t.Fatalf("projects = %v, want only the edited one left", left)
	}
	again, _ := stageFile(t, e, "project", importProjectHeader+"\nP-1,Edited by a rep,"+w.acme+",,,,,\nP-2,Left alone,"+w.acme+",,,,,\n")
	if report := commitFile(t, e, again); report.Disposition.Created != 1 {
		t.Fatalf("the same file after the undo = %+v issues %+v, want the project the undo archived created again", report.Disposition, report.Issues)
	}
}

// A project names a company the same upload creates first. The test run has no
// id for that company yet, and the commit lands both.
func TestAProjectInTheSameZipAsItsCompanyIsPreviewedAsTheCommitLandsIt(t *testing.T) {
	e := setupImportApp(t)
	files := map[string]string{
		"companies.csv": "ref,name\nCO-1,Zip Works GmbH\n",
		"projects.csv":  "ref,name,company_ref,phase\nPR-1,Zipped,CO-1,delivering\nPR-2,Nobody,CO-9,\nPR-3,Closed,CO-1,closed\n",
	}
	parts := []AnyMap{
		bundleParts()[1],
		{"file": "projects.csv", "object": "project", "mapping": AnyMap{"ref": "ref", "name": "name", "company_ref": "company_ref", "phase": "phase"}},
	}
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if committed.Disposition.Created != 2 || committed.Disposition.Skipped != 2 {
		t.Fatalf("commit = %+v issues %+v, want the company and one project created, two projects skipped", committed.Disposition, committed.Issues)
	}
	wantSameAnswers(t, preview, committed)
	if zipped := importedProjects(t, e)["Zipped"]; zipped.Phase != "delivering" || zipped.CompanyID == "" {
		t.Fatalf("project = %+v, want it on the company the same ZIP made", zipped)
	}
}

// An agent's pasted files take the same road, with the company and the project
// in one call.
func TestAnAgentImportsAProjectAndItsCompanyFromOneCall(t *testing.T) {
	startDay, _ := projectDays(t)
	d := newTwoDoorsWith(t, "import-project-tool", nil, "read", "write")
	var preview struct {
		Run struct {
			RunID string `json:"run_id"`
		} `json:"run"`
	}
	decode(t, d.tool(t, "preview_import", map[string]any{
		"source_name": "old-crm",
		"files": []map[string]any{
			{
				"name": "accounts.csv", "object": "company", "csv": "ref,name\nCO-1,Toolby GmbH\n",
				"mapping": map[string]string{"ref": "ref", "name": "display_name"},
			},
			{
				"name": "projects.csv", "object": "project", "csv": "ref,name,company,start\nPR-1,Tool rollout,CO-1," + startDay + "\n",
				"mapping": map[string]string{"ref": "ref", "name": "name", "company": "company_ref", "start": "started_at"},
			},
		},
	}), &preview)
	importjobtest.Drain(t, d.e)
	d.tool(t, "commit_import", map[string]any{"run_id": preview.Run.RunID})
	importjobtest.Drain(t, d.e)
	var read struct {
		State  string `json:"state"`
		Report *struct {
			Disposition struct {
				Created int `json:"created"`
			} `json:"disposition"`
			Issues []json.RawMessage `json:"issues"`
		} `json:"report"`
	}
	decode(t, d.tool(t, "read_import", map[string]any{"view": "report", "run_id": preview.Run.RunID}), &read)
	if read.State != "complete" || read.Report == nil || read.Report.Disposition.Created != 2 || len(read.Report.Issues) != 0 {
		t.Fatalf("the finished run reads %+v, want the company and the project created with no issues", read)
	}
	if project := importedProjects(t, d.e)["Tool rollout"]; project.CompanyID == "" || project.StartedAt == nil || *project.StartedAt != startDay {
		t.Fatalf("project = %+v, want it on the company of the same call, with its start day", project)
	}
}

// A deal named by its ref in one row and by its id in another is one deal.
// The test run refuses the second project as the commit does.
func TestOneDealNamedByRefAndByIDStartsOnlyOneProject(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	importDeals(t, e, map[string]string{
		"companies.csv": "ref,name\nCO-1,Twice GmbH\n",
		"deals.csv":     "ref,name,pipeline,stage,company_ref\nD-77,Named twice," + pipeline + "," + open + ",CO-1\n",
	}, append(bundleParts()[1:], dealImportParts("ref,name,pipeline,stage,company_ref")...))
	named := dealNamed(t, e, "Named twice")
	dealID, company := named.ID, *named.CompanyID
	files := map[string]string{"projects.csv": "ref,name,company_ref,deal_ref\n" +
		"P-1,By ref," + company + ",D-77\nP-2,By id," + company + "," + dealID + "\n"}
	parts := []AnyMap{{"file": "projects.csv", "object": "project", "mapping": AnyMap{"ref": "ref", "name": "name", "company_ref": "company_ref", "deal_ref": "deal_ref"}}}
	run := stageBundle(t, e, uploadZip(t, e, "projects.zip", zipBytes(t, files)).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	if preview.Disposition.Created != 1 || preview.Disposition.Skipped != 1 {
		t.Fatalf("test run = %+v issues %+v, want one project created and the second refused", preview.Disposition, preview.Issues)
	}
	approveAndRead(t, e, run.ID)
	wantSameAnswers(t, preview, readImportReport(t, e, run.ID))
}

// A company the upload creates first never matches a deal that already names
// another company, in the test run as in the commit.
func TestAProjectOnAnUploadsNewCompanyIsCheckedAgainstAnExistingDeal(t *testing.T) {
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	files := map[string]string{
		"companies.csv": "ref,name\nCO-1,Fresh GmbH\n",
		"projects.csv":  "ref,name,company_ref,deal_ref\nPR-1,Wrong deal,CO-1," + w.otherDeal + "\nPR-2,No deal,CO-1,\n",
	}
	parts := []AnyMap{
		bundleParts()[1],
		{"file": "projects.csv", "object": "project", "mapping": AnyMap{"ref": "ref", "name": "name", "company_ref": "company_ref", "deal_ref": "deal_ref"}},
	}
	run := stageBundle(t, e, uploadZip(t, e, "crm.zip", zipBytes(t, files)).SourceRef, "crm", parts)
	preview := readImportReport(t, e, run.ID)
	if preview.Disposition.Created != 2 || preview.Disposition.Skipped != 1 {
		t.Fatalf("test run = %+v issues %+v, want the company and one project created, the project on the other company's deal refused", preview.Disposition, preview.Issues)
	}
	approveAndRead(t, e, run.ID)
	wantSameAnswers(t, preview, readImportReport(t, e, run.ID))
}

// A row ref is kept in the import's own identity table, never on the project.
// Another import that uses the same ref cannot collide with a project it did not
// make, so no import learns that a ref is taken.
func TestTwoImportsUsingOneRowRefDoNotCollide(t *testing.T) {
	e := setupImportApp(t)
	w := newProjectWorld(t, e)
	for _, source := range []string{"first-crm", "second-crm"} {
		files := map[string]string{"projects.csv": "ref,name,company_ref\nP-1,From " + source + "," + w.acme + "\n"}
		parts := []AnyMap{{"file": "projects.csv", "object": "project", "mapping": AnyMap{"ref": "ref", "name": "name", "company_ref": "company_ref"}}}
		run := stageBundle(t, e, uploadZip(t, e, source+".zip", zipBytes(t, files)).SourceRef, source, parts)
		preview := readImportReport(t, e, run.ID)
		approveAndRead(t, e, run.ID)
		committed := readImportReport(t, e, run.ID)
		if committed.Disposition.Created != 1 || len(committed.Issues) != 0 {
			t.Fatalf("%s: commit = %+v issues %+v, want its project created with no issue", source, committed.Disposition, committed.Issues)
		}
		wantSameAnswers(t, preview, committed)
	}
	if projects := importedProjects(t, e); len(projects) != 2 {
		t.Fatalf("projects = %v, want one from each import", projects)
	}
}
