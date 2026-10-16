// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

func unknownOwnerIssue(file, email string, line int) crmcontracts.ImportRowIssue {
	kind := crmcontracts.ImportProblemKind(migration.ProblemUnknownOwner)
	code := crmcontracts.ImportReasonCode(migration.ReasonOwnerUnknown)
	return crmcontracts.ImportRowIssue{File: &file, Value: &email, Line: &line, Kind: &kind, ReasonCode: &code}
}

func TestRowsNamingOneUnknownOwnerAreOneWarningWithTheirCount(t *testing.T) {
	brokenKind := crmcontracts.ImportProblemKind(migration.ProblemBrokenLink)
	issues := []crmcontracts.ImportRowIssue{
		unknownOwnerIssue("contacts.csv", "jo@example.com", 2),
		{Kind: &brokenKind, Reason: "kept as it is"},
		unknownOwnerIssue("contacts.csv", " JO@example.com", 5),
		unknownOwnerIssue("contacts.csv", "li@example.com", 6),
		unknownOwnerIssue("companies.csv", "jo@example.com", 3),
		unknownOwnerIssue("contacts.csv", "jo@example.com", 9),
	}
	got := collapseUnknownOwners(issues)
	if len(got) != 4 {
		t.Fatalf("got %d issues, want the broken link and one warning per file and address: %+v", len(got), got)
	}
	want := map[string]string{"contacts.csv/jo@example.com": "3", "contacts.csv/li@example.com": "1", "companies.csv/jo@example.com": "1"}
	for _, issue := range got {
		if issue.Kind == nil || string(*issue.Kind) != migration.ProblemUnknownOwner {
			continue
		}
		key := *issue.File + "/" + *issue.Value
		count := (*issue.Params)["rows"]
		if want[key] != count {
			t.Errorf("%s: count %q, want %q", key, count, want[key])
		}
		if issue.Reason == "" || *issue.ReasonCode != crmcontracts.ImportReasonCode(migration.ReasonOwnerUnknown) {
			t.Errorf("%s: reason %q code %v, want the owner_unknown sentence with the count", key, issue.Reason, issue.ReasonCode)
		}
	}
	if *got[0].Line != 2 {
		t.Errorf("the warning sits on line %d, want the first row that names the address", *got[0].Line)
	}
}

// A clean file has no issues, and the wire says so with an empty list: a
// null would break every reader that counts them.
func TestACleanReportSendsAnEmptyIssueList(t *testing.T) {
	report := migration.Report{Objects: []migration.ObjectReport{{Object: migration.ObjectCompany, MirrorCount: 2, Unchanged: 2}}}
	got := toContractImportReport(migration.Run{
		Status: migration.StatusAwaitingApproval, Report: &report,
		Mapping: &migration.RunMapping{Object: migration.ObjectCompany},
	})
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), `"issues":[]`) {
		t.Fatalf("report wire = %s, want issues as an empty list", wire)
	}
	if collapseUnknownOwners(nil) == nil {
		t.Fatal("collapsing no issues answered nil")
	}
}
