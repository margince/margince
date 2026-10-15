// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
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
