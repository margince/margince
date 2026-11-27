// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// A report stored without the line still names it: the id of a skip the source
// disclosed spells "line N". A carried line wins over the id.
func TestAStoredReportsSkipStillNamesItsLine(t *testing.T) {
	if got := lineOf(migration.SkippedRow{ExternalID: "line 7"}); got != 7 {
		t.Errorf("line = %d, want 7 — a report written before the line was carried must still read", got)
	}
	if got := lineOf(migration.SkippedRow{ExternalID: "b@x.test", Line: 4}); got != 4 {
		t.Errorf("line = %d, want the carried 4", got)
	}
	// A mirror source's rows carry the incumbent's id, so only the whole id is
	// read: "line 7 of the export" names no line.
	for _, foreign := range []string{"line 7 of the export", "outline 7", "line seven", "line"} {
		if got := lineOf(migration.SkippedRow{ExternalID: foreign}); got != 0 {
			t.Errorf("lineOf(%q) = %d, want 0 — that id is not this source's disclosure", foreign, got)
		}
	}
}

// A file whose key column holds the text a source disclosure uses keeps both
// rows in the report. One row skipped twice is still one issue.
func TestARowKeyedLikeADisclosureIsNotFoldedOntoIt(t *testing.T) {
	var issues []crmcontracts.ImportRowIssue
	seen := map[string]bool{}
	lead := migration.ObjectReport{Object: migration.ObjectLead}
	AppendIssues(&issues, seen, IssueKey, lead, []migration.SkippedRow{
		// What the source writes for a row with no key at all, on line 7.
		{ExternalID: "line 7", Line: 7, Reason: "no key"},
		// A row on line 12 whose key column really does hold that text.
		{ExternalID: "line 7", Line: 12, Reason: "not-a-uuid"},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %+v, want both rows — they are two rows a contact must go fix", issues)
	}
	AppendIssues(&issues, seen, IssueKey, lead, []migration.SkippedRow{{ExternalID: "line 7", Line: 7, Reason: "no key"}})
	if len(issues) != 2 {
		t.Errorf("the same row was reported twice; the dry run's skip and the commit's are one row")
	}
	// Two files of a ZIP each have their own line 7.
	AppendIssues(&issues, seen, IssueKey, migration.ObjectReport{Object: migration.ObjectCompany, File: "companies.csv"},
		[]migration.SkippedRow{{ExternalID: "line 7", Line: 7, Reason: "no key"}})
	if len(issues) != 3 || issues[2].File == nil || *issues[2].File != "companies.csv" {
		t.Errorf("issues = %+v, want a third issue naming companies.csv", issues)
	}
}

func TestAProblemOfAWholeFileIsReportedWithNoLine(t *testing.T) {
	var issues []crmcontracts.ImportRowIssue
	companies := migration.ObjectReport{Object: migration.ObjectCompany, File: "companies.csv"}
	AppendIssues(&issues, map[string]bool{}, WarningKey, companies, []migration.SkippedRow{
		{Kind: string(crmcontracts.ImportProblemKindNewTag), Value: "vip", Reason: "a new tag"},
		{Kind: string(crmcontracts.ImportProblemKindNewTag), Value: "partner", Reason: "a new tag"},
	})
	if len(issues) != 2 {
		t.Fatalf("issues = %+v, want one per new tag", issues)
	}
	for _, issue := range issues {
		if issue.Line != nil {
			t.Errorf("issue %q has line %d, want none: a new tag belongs to the file, not a row", *issue.Value, *issue.Line)
		}
	}
}
