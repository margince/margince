// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Importing a file of PEOPLE, end to end.
//
// Split from csvimport_integration_test.go on the object rather than the line
// count: a person is the one import object whose identity lives in a child table
// and whose rows carry an edge to another record, so every promise here is one
// the company and lead paths never have to make.

import (
	"net/http"
	"strings"
	"testing"
)

// The other half of the same promise: a file of people the business already
// knows imports as PEOPLE, through the same dry-run-then-approve flow.
//
// Three things are asserted together because each one failing alone would still
// look like a working import: the dry run writes nothing, the commit creates
// people rather than leads, and re-running the identical file converges instead
// of reporting an update forever. That last one is the defect a person object
// introduces on its own — an email is a child row, so a comparison that reads
// the record's flat fields finds no `email` and calls every row changed.
func TestCSVImportOfKnownPeopleCreatesPeople(t *testing.T) {
	e := setupImportApp(t)
	peopleBefore := importedPersonCount(t, e)
	leadsBefore := leadCount(t, e)

	profile, status := uploadCSV(t, e, "person", prospectCSV)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	if profile.RowsProfiled != 3 {
		t.Fatalf("profile = %+v, want 3 rows", profile)
	}

	run, runStatus := createRunWithMapping(t, e, "person", profile.SourceRef,
		map[string]string{"Email": "email", "Full Name": "full_name", "Title": "title"})
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}
	if got := importedPersonCount(t, e); got != peopleBefore {
		t.Fatalf("the dry run created %d people; a validation pass writes nothing", got-peopleBefore)
	}

	var report importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("report → %d, want 200", status)
	}
	if report.Disposition.Created != 3 {
		t.Fatalf("report = %+v, want 3 rows the commit will create", report)
	}

	var approved importRunDTO
	if status := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); status != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", status)
	}
	if got := importedPersonCount(t, e); got != peopleBefore+3 {
		t.Fatalf("people = %d, was %d — a `person` run creates people", got, peopleBefore)
	}
	if got := leadCount(t, e); got != leadsBefore {
		t.Fatalf("leads = %d, was %d — a `person` run does not reach the lead table", got, leadsBefore)
	}

	// The same file again: every row is already here and unchanged, so the
	// commit has nothing to do. An `email` the comparison cannot read would
	// report three updates instead.
	second, secondStatus := uploadCSV(t, e, "person", prospectCSV)
	if secondStatus != http.StatusOK {
		t.Fatalf("second upload → %d, want 200", secondStatus)
	}
	rerun, rerunStatus := createRunWithMapping(t, e, "person", second.SourceRef,
		map[string]string{"Email": "email", "Full Name": "full_name", "Title": "title"})
	if rerunStatus != http.StatusAccepted {
		t.Fatalf("create re-run → %d, want 202", rerunStatus)
	}
	var rereport importReportDTO
	if status := e.Call(t, http.MethodGet, "/v1/imports/"+rerun.ID+"/report", nil, nil, &rereport); status != http.StatusOK {
		t.Fatalf("re-report → %d, want 200", status)
	}
	if rereport.Disposition.Created != 0 || rereport.Disposition.Updated != 0 {
		t.Fatalf("re-run = %+v, want nothing created and nothing updated — the file is unchanged", rereport.Disposition)
	}
}

// A row whose address a previous run of this importer already landed: preview
// and commit must give the SAME answer, and here that answer is an update.
//
// The email is the source key, so the identity map recognises the row and the
// person is corrected rather than duplicated — which is the whole point of
// keying a person file on its addresses. What must never happen is the preview
// promising one disposition and the commit producing another.
func TestCSVImportOfAClaimedEmailPreviewsWhatTheCommitDoes(t *testing.T) {
	e := setupImportApp(t)

	first, status := uploadCSV(t, e, "person", prospectCSV)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	mapping := map[string]string{"Email": "email", "Full Name": "full_name", "Title": "title"}
	run, runStatus := createRunWithMapping(t, e, "person", first.SourceRef, mapping)
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}
	var approved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}

	// A second file naming the SAME address under a different name. The source
	// key is the email, so the identity map recognises the row and this is an
	// update — which is the right answer, and the one the commit must also give.
	const clash = "Email,Full Name,Title\n" +
		"ada@lovelace.example,Ada Byron,Countess\n" +
		"joan@clarke.example,Joan Clarke,Cryptanalyst\n"
	second, secondStatus := uploadCSV(t, e, "person", clash)
	if secondStatus != http.StatusOK {
		t.Fatalf("second upload → %d, want 200", secondStatus)
	}
	rerun, rerunStatus := createRunWithMapping(t, e, "person", second.SourceRef, mapping)
	if rerunStatus != http.StatusAccepted {
		t.Fatalf("create re-run → %d, want 202", rerunStatus)
	}

	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+rerun.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("report → %d, want 200", s)
	}
	if report.Disposition.Created != 1 || report.Disposition.Updated != 1 {
		t.Fatalf("preview = %+v, want 1 create and 1 update for the address already held", report.Disposition)
	}

	var reapproved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+rerun.ID+"/approve", nil, nil, &reapproved); s != http.StatusAccepted {
		t.Fatalf("approve re-run → %d, want 202", s)
	}
	var after importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+rerun.ID+"/report", nil, nil, &after); s != http.StatusOK {
		t.Fatalf("report after commit → %d, want 200", s)
	}
	if after.Disposition.Created != 1 || after.Disposition.Updated != 1 {
		t.Fatalf("commit = %+v, want the same 1 create and 1 update the preview promised", after.Disposition)
	}
}

// A contact file naming employers links each person to their company, and says
// what it will do BEFORE anyone approves it.
//
// The whole flow in one test, because each half passing alone would still be a
// broken feature: the preview must report how many links will actually resolve
// (not how many the file asks for), and the commit must write exactly those.
//
// The companies arrive the way real ones do — created directly, not by a
// previous CSV run — because that is the case the importer's identity map
// cannot help with, and the one every real migration hits.
func TestCSVImportLinksPeopleToTheirEmployers(t *testing.T) {
	e := setupImportApp(t)
	for _, name := range []string{"Analytical Engines", "Bletchley Ltd"} {
		if status := e.Call(t, http.MethodPost, "/v1/organizations",
			map[string]any{"display_name": name}, nil, nil); status != http.StatusCreated {
			t.Fatalf("creating %q → %d, want 201", name, status)
		}
	}

	// Three people: two naming a company that exists, one naming a company
	// nobody has imported.
	const contacts = "Email,Full Name,Company\n" +
		"ada@x.test,Ada Lovelace,Analytical Engines\n" +
		"joan@x.test,Joan Clarke,Bletchley Ltd\n" +
		"katherine@x.test,Katherine Johnson,Langley Research\n"

	profile, status := uploadCSV(t, e, "person", contacts)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	mapping := map[string]string{
		"Email": "email", "Full Name": "full_name", "Company": "organization_name",
	}
	run, runStatus := createRunWithMapping(t, e, "person", profile.SourceRef, mapping)
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}

	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("report → %d, want 200", s)
	}
	if report.Links == nil {
		t.Fatal("the report carries no links section, so a person approving it is told nothing about employers")
	}
	// The file asks for three links and two of the companies are here, so the
	// preview says three offered with one named as unresolvable. Both numbers
	// matter: "3 asked for, 1 we cannot make" is the decision a person is
	// actually taking, and reporting only the resolvable two would hide the
	// company they need to go and import.
	if report.Links.Offered != 3 {
		t.Fatalf("links.offered = %d, want the 3 the file names", report.Links.Offered)
	}
	if report.Links.Applied != 0 {
		t.Fatalf("links.applied = %d on a dry run, want 0 — a dry run writes nothing", report.Links.Applied)
	}
	if len(report.Links.Unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want the one company that is not in the CRM", report.Links.Unresolved)
	}
	if !strings.Contains(report.Links.Unresolved[0].Reason, "Langley Research") {
		t.Fatalf("reason = %q, want it to name the company that was not found", report.Links.Unresolved[0].Reason)
	}

	var approved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}

	var after importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &after); s != http.StatusOK {
		t.Fatalf("report after commit → %d, want 200", s)
	}
	if after.Links == nil || after.Links.Applied != 2 {
		t.Fatalf("links after commit = %+v, want the 2 the preview promised", after.Links)
	}
	// Every person lands, including the one whose employer was not found: an
	// unresolvable company is a missing link, never a missing contact.
	if got := importedPersonCount(t, e); got != 3 {
		t.Fatalf("people = %d, want all 3 — a row's employer not resolving must not lose the person", got)
	}
}

// A company whose legal form differs from the file's is NOT the same company.
//
// This is the finding that makes the difference between a review queue and a
// write. NormalizeOrgName strips legal suffixes, so `Acme Inc` and `Acme GmbH`
// normalize alike — right for asking a human "are these two the same?", wrong
// for deciding it. Two different legal entities linked by a guess is a wrong
// answer nobody sees; a missing link is one the report names.
func TestCSVImportDoesNotLinkAcrossLegalForms(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Northwind GmbH"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the company → %d, want 201", status)
	}

	const contacts = "Email,Full Name,Company\nada@x.test,Ada Lovelace,Northwind Inc\n"
	profile, status := uploadCSV(t, e, "person", contacts)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, runStatus := createRunWithMapping(t, e, "person", profile.SourceRef, map[string]string{
		"Email": "email", "Full Name": "full_name", "Company": "organization_name",
	})
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}

	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("report → %d, want 200", s)
	}
	if report.Links == nil || len(report.Links.Unresolved) != 1 {
		t.Fatalf("links = %+v, want the differently-spelled company reported unresolved", report.Links)
	}
	if !strings.Contains(report.Links.Unresolved[0].Reason, "Northwind Inc") {
		t.Fatalf("reason = %q, want it to name the company as the FILE spells it",
			report.Links.Unresolved[0].Reason)
	}
}

// A row the commit will refuse takes its employer link down with it.
//
// The person is never created, so there is nobody to link. A preview counting
// that link as resolvable would promise something the commit cannot do, which is
// the disagreement the whole dry run exists to prevent.
func TestCSVImportDoesNotPromiseLinksForRowsThatWillNotLand(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Analytical Engines"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the company → %d, want 201", status)
	}

	// The second row's address is malformed, so the store refuses it before the
	// write transaction opens — and the preview already knows that.
	const contacts = "Email,Full Name,Company\n" +
		"ada@x.test,Ada Lovelace,Analytical Engines\n" +
		"not-an-address,Broken Row,Analytical Engines\n"
	profile, status := uploadCSV(t, e, "person", contacts)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, runStatus := createRunWithMapping(t, e, "person", profile.SourceRef, map[string]string{
		"Email": "email", "Full Name": "full_name", "Company": "organization_name",
	})
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}

	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("report → %d, want 200", s)
	}
	if report.Links == nil || report.Links.Offered != 2 {
		t.Fatalf("links = %+v, want both rows counted as asking for a link", report.Links)
	}
	if len(report.Links.Unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want the refused row's link reported as unmakeable",
			report.Links.Unresolved)
	}

	var approved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}
	var after importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &after); s != http.StatusOK {
		t.Fatalf("report after commit → %d, want 200", s)
	}
	if after.Links == nil || after.Links.Applied != 1 {
		t.Fatalf("applied = %+v, want the one link the preview promised", after.Links)
	}
}
