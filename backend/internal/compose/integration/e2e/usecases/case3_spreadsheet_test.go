// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// CASE 3 — A spreadsheet arrives.
//
// The prompts, in the order a real user says them:
//
//	Got this list from the trade fair. Can you get it into the CRM?
//	I think I sent you that already, do it again to be safe
//	One correction on that list — Rostock should be Hamburg and they are
//	bigger than I said, 201-500 people
//
// The second line is the whole case. It is what a user does when they are not
// sure whether something worked, and the correct outcome is that nothing
// happens twice.
//
// A blob store is wired because the import genuinely needs one: profiling
// STORES the bytes, and both the dry run and the commit reopen the stored
// source. An in-memory store is enough — no MinIO, no network.
//
// NOT covered here: criterion 8, undo. There is no undo tool on this surface;
// it is human-only REST, and a scenario driving it over the cookie client would
// be testing the app rather than the journey an assistant walks.

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
)

// The file from the trade fair. Four companies, and one row whose id names
// nothing — so criteria 6 and 7 have something to catch.
const tradeFairCSV = `name,city,size,country
Nordwind Logistik GmbH,Rostock,51-200,DE
Elbmarsch Systeme AG,Hamburg,11-50,DE
Rheinpark Automation KG,Köln,201-500,DE
Alpenblick Sensorik GmbH,München,11-50,DE
`

// theCorrection is the same file with one row changed: the city and the size,
// exactly as the user described them. Everything else is byte-identical, so an
// import that touched anything but Nordwind is doing work nobody asked for.
// The country column is ABSENT, which is the point: criterion 5 is that a
// field the correction never mentions is left alone. An earlier version of this
// file kept the column and supplied DE, so it proved only that writing DE over
// DE leaves DE — a mutation replacing every address component would have passed.
const theCorrectionCSV = `name,city,size
Nordwind Logistik GmbH,Hamburg,201-500
Elbmarsch Systeme AG,Hamburg,11-50
Rheinpark Automation KG,Köln,201-500
Alpenblick Sensorik GmbH,München,11-50
`

// importOutcome is what one preview-then-commit produced.
type importOutcome struct {
	runID  string
	report crmcontracts.ImportRunReport
}

// previewImport runs the dry pass and returns what it says WILL happen.
//
// The mapping names only the columns the file actually carries. A mapping that
// named a column the file lacks would be a different test — and the correction
// file deliberately lacks one.
func (s *scenario) previewImport(t *testing.T, csv string) agents.ImportPreviewResult {
	t.Helper()
	mapping := map[string]string{
		"name": "display_name", "city": "address.city", "size": "size_band",
	}
	if strings.Contains(strings.SplitN(csv, "\n", 2)[0], "country") {
		mapping["country"] = "address.country"
	}
	got := s.MCP.CallOK(t, "preview_import", map[string]any{
		"object": "organization", "csv": csv, "mapping": mapping,
	})
	var preview agents.ImportPreviewResult
	got.JSON(t, &preview)
	return preview
}

// readReport reads a run's report, which is the same shape before and after the
// commit — so what WILL happen and what DID compares like with like.
func (s *scenario) readReport(t *testing.T, runID string) crmcontracts.ImportRunReport {
	t.Helper()
	got := s.MCP.CallOK(t, "read_import_report", map[string]any{"run_id": runID})
	var report agents.ImportReportResult
	got.JSON(t, &report)
	return report.Report
}

// runImport previews, commits and reads back what actually happened.
func (s *scenario) runImport(t *testing.T, csv string) importOutcome {
	t.Helper()
	preview := s.previewImport(t, csv)
	if preview.Run.RunID == "" {
		t.Fatalf("case 3: the preview produced no run")
	}
	// commit_import is auto-execute: importing a file the user handed over is
	// not a decision a second human makes.
	s.MCP.CallOK(t, "commit_import", map[string]any{"run_id": preview.Run.RunID})
	return importOutcome{runID: preview.Run.RunID, report: s.readReport(t, preview.Run.RunID)}
}

// TestCase3ThePreviewTellsTheTruth pins criteria 1 and 2.
//
// The numbers are shown before anything is written, and what happens matches
// them exactly. A preview that is roughly right is a preview nobody can approve
// against.
func TestCase3ThePreviewTellsTheTruth(t *testing.T) {
	s := bootWithImports(t)

	preview := s.previewImport(t, tradeFairCSV)
	promised := s.readReport(t, preview.Run.RunID)
	if promised.Disposition.Created != 4 {
		t.Fatalf("case 3 criterion 1: the dry run promises %d creations for a four-company file",
			promised.Disposition.Created)
	}
	// Nothing is written yet. A preview that had already created the rows
	// would make the count true and the promise meaningless.
	if s.countRows(t, `SELECT count(*) FROM organization WHERE display_name LIKE '%GmbH'
		OR display_name LIKE '%AG' OR display_name LIKE '%KG'`) != 0 {
		t.Fatalf("case 3 criterion 1: the dry run already wrote companies, so the numbers it " +
			"showed were a description of the past rather than a promise")
	}

	s.MCP.CallOK(t, "commit_import", map[string]any{"run_id": preview.Run.RunID})
	happened := s.readReport(t, preview.Run.RunID)

	if happened.Disposition.Created != promised.Disposition.Created {
		t.Fatalf("case 3 criterion 2: the preview promised %d created and %d happened",
			promised.Disposition.Created, happened.Disposition.Created)
	}
	if happened.Disposition.Updated != promised.Disposition.Updated {
		t.Fatalf("case 3 criterion 2: the preview promised %d updated and %d happened",
			promised.Disposition.Updated, happened.Disposition.Updated)
	}
	// Every row, bound to its own values. An aggregate of four proves four rows
	// appeared; it does not prove they are the four the file named, with the
	// data the file carried.
	for _, want := range []struct{ name, city, size string }{
		{"Nordwind Logistik GmbH", "Rostock", "51-200"},
		{"Elbmarsch Systeme AG", "Hamburg", "11-50"},
		{"Rheinpark Automation KG", "Köln", "201-500"},
		{"Alpenblick Sensorik GmbH", "München", "11-50"},
	} {
		if n := s.countRows(t, `SELECT count(*) FROM organization
			WHERE display_name = $1 AND address_city = $2 AND size_band = $3`,
			want.name, want.city, want.size); n != 1 {
			t.Fatalf("case 3 criterion 2: the file says %s is in %s at %s, and %d rows match",
				want.name, want.city, want.size, n)
		}
	}
}

// TestCase3ImportingTheSameFileTwiceChangesNothing pins criterion 3, the single
// most important assertion in this case.
//
// "I think I sent you that already, do it again to be safe" must be safe.
func TestCase3ImportingTheSameFileTwiceChangesNothing(t *testing.T) {
	s := bootWithImports(t)

	first := s.runImport(t, tradeFairCSV)
	if first.report.Disposition.Created != 4 {
		t.Fatalf("case 3: the first import created %d companies, want 4",
			first.report.Disposition.Created)
	}

	second := s.runImport(t, tradeFairCSV)
	if second.report.Disposition.Created != 0 {
		t.Fatalf("case 3 criterion 3: re-importing the identical file created %d more companies",
			second.report.Disposition.Created)
	}
	if second.report.Disposition.Updated != 0 {
		t.Fatalf("case 3 criterion 3: re-importing the identical file updated %d companies; every "+
			"value was already equal and updating them writes work that never happened into the "+
			"audit trail", second.report.Disposition.Updated)
	}
	if second.report.Disposition.Unchanged != 4 {
		t.Fatalf("case 3 criterion 3: the second run reports %d unchanged, want all 4",
			second.report.Disposition.Unchanged)
	}
	if total := s.countRows(t, `SELECT count(*) FROM organization
		WHERE display_name IN ('Nordwind Logistik GmbH','Elbmarsch Systeme AG',
		                       'Rheinpark Automation KG','Alpenblick Sensorik GmbH')`); total != 4 {
		t.Fatalf("case 3 criterion 3: the CRM holds %d of the file's companies after importing it "+
			"twice, want 4", total)
	}

	// NOTHING WAS WRITTEN, not merely nothing was counted.
	//
	// `unchanged` is a residual — rows read minus created, updated and skipped —
	// so a build that rewrote every row with its own values and reported zero
	// updates would produce identical counts. The row version is what a write
	// actually leaves behind: it is bumped by every real update, so four rows
	// still at version 1 is the claim the disposition alone cannot make.
	if untouched := s.countRows(t, `SELECT count(*) FROM organization
		WHERE display_name IN ('Nordwind Logistik GmbH','Elbmarsch Systeme AG',
		                       'Rheinpark Automation KG','Alpenblick Sensorik GmbH')
		  AND version = 1`); untouched != 4 {
		t.Fatalf("case 3 criterion 3: only %d of the 4 companies are still at version 1 — the "+
			"second import reported no updates and rewrote rows anyway, which puts work that "+
			"never happened into the audit trail", untouched)
	}
}

// TestCase3ChangingOneRowUpdatesOneRecord pins criteria 4 and 5.
//
// Not all of them, and not a duplicate. And the country column, which the
// correction did not touch, is left exactly as it was.
func TestCase3ChangingOneRowUpdatesOneRecord(t *testing.T) {
	s := bootWithImports(t)
	s.runImport(t, tradeFairCSV)

	corrected := s.runImport(t, theCorrectionCSV)
	if corrected.report.Disposition.Updated != 1 {
		t.Fatalf("case 3 criterion 4: one row changed and %d records were updated",
			corrected.report.Disposition.Updated)
	}
	if corrected.report.Disposition.Created != 0 {
		t.Fatalf("case 3 criterion 4: correcting a row created %d companies — a correction that "+
			"creates is a duplicate", corrected.report.Disposition.Created)
	}
	if corrected.report.Disposition.Unchanged != 3 {
		t.Fatalf("case 3 criterion 4: %d of the other three rows were reported unchanged",
			corrected.report.Disposition.Unchanged)
	}

	// BOTH halves of the correction landed. The user said two things — the city
	// and the size — and a city change alone is enough to report one update, so
	// asserting only the city would pass while losing half of what was asked.
	if city := s.readStringWhere(t,
		`SELECT coalesce(address_city, '') FROM organization WHERE display_name = $1`,
		"Nordwind Logistik GmbH"); city != "Hamburg" {
		t.Fatalf("case 3 criterion 4: Nordwind should have moved to Hamburg and sits in %q", city)
	}
	if size := s.readStringWhere(t,
		`SELECT coalesce(size_band, '') FROM organization WHERE display_name = $1`,
		"Nordwind Logistik GmbH"); size != "201-500" {
		t.Fatalf("case 3 criterion 4: the correction said 201-500 people and the record reads %q — "+
			"one update was reported and half the correction was dropped", size)
	}
	// Criterion 5: a field the correction's file does not CARRY is untouched.
	// The original import set DE; the correction has no country column at all.
	if country := s.readStringWhere(t,
		`SELECT coalesce(address_country, '') FROM organization WHERE display_name = $1`,
		"Nordwind Logistik GmbH"); country != "DE" {
		t.Fatalf("case 3 criterion 5: the correction file carries no country column and the "+
			"country now reads %q — an import must not blank what it was not told about", country)
	}
}

// TestCase3ABadRowIsSkippedWithAReasonAndTheRestStillImport pins criteria 6
// and 7.
//
// One broken line does not fail the file, and it does not silently create a
// duplicate either. The reason names the LINE, because a user has to be able to
// find and fix it.
func TestCase3ABadRowIsSkippedWithAReasonAndTheRestStillImport(t *testing.T) {
	s := bootWithImports(t)

	// The third data row carries no name at all — nothing the import can file.
	const withABadRow = `name,city,size,country
Nordwind Logistik GmbH,Rostock,51-200,DE
Elbmarsch Systeme AG,Hamburg,11-50,DE
,Köln,201-500,DE
Alpenblick Sensorik GmbH,München,11-50,DE
`
	outcome := s.runImport(t, withABadRow)

	if outcome.report.Disposition.Created != 3 {
		t.Fatalf("case 3 criterion 6: one row of five is unusable and %d companies were created, "+
			"want the other 3 — a broken line must not fail the file",
			outcome.report.Disposition.Created)
	}
	if len(outcome.report.Issues) == 0 {
		t.Fatalf("case 3 criterion 6: a row was dropped and no issue was reported, so a file " +
			"half-ignored arrives under a success message")
	}
	// Criterion 7: the issue names THE row that was bad, not merely a row.
	//
	// The blank name is on the fourth line of the file, counting the header as
	// line 1. An implementation reporting line 2 for everything satisfies "at
	// least line 2" while sending the user to the wrong row — and a generic
	// reason tells them nothing about what to fix when they get there.
	const badLine = 4
	found := false
	for _, issue := range outcome.report.Issues {
		if strings.TrimSpace(issue.Reason) == "" {
			t.Fatalf("case 3 criterion 6: line %d was refused with no reason", issue.Line)
		}
		if issue.Line == badLine {
			found = true
			if !mentionsTheMissingName(issue) {
				t.Fatalf("case 3 criterion 7: line %d was refused because it carries no company "+
					"name, and the reason given is %q — a user cannot tell what to fix",
					issue.Line, issue.Reason)
			}
		}
	}
	if !found {
		t.Fatalf("case 3 criterion 7: the unusable row is line %d of the file and the issues "+
			"reported are %v — a user sent to the wrong row cannot fix it",
			badLine, linesOf(outcome.report.Issues))
	}
}

// readStringWhere reads one column by a predicate other than an id.
func (s *scenario) readStringWhere(t *testing.T, sql, arg string) string {
	t.Helper()
	var out string
	if err := queryRow(t, s, sql, &out, arg); err != nil {
		t.Fatalf("reading: %v\n%s", err, sql)
	}
	return out
}

// mentionsTheMissingName says whether an issue explains what was wrong with the
// row, rather than merely that something was.
func mentionsTheMissingName(issue crmcontracts.ImportRowIssue) bool {
	haystack := strings.ToLower(issue.Reason)
	if issue.Column != nil {
		haystack += " " + strings.ToLower(*issue.Column)
	}
	for _, word := range []string{"name", "blank", "empty", "missing", "required"} {
		if strings.Contains(haystack, word) {
			return true
		}
	}
	return false
}

// linesOf names which lines were reported, for a failure worth reading.
func linesOf(issues []crmcontracts.ImportRowIssue) []int {
	out := make([]int, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issue.Line)
	}
	return out
}
