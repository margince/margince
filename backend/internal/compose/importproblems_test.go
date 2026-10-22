// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestProblemsCSVDefusesAFormulaInAValue(t *testing.T) {
	rep := crmcontracts.ImportRunReport{Issues: []crmcontracts.ImportRowIssue{
		{Line: new(3), Reason: "not a number", Column: new("Value"), Value: new(`=HYPERLINK("x")`), File: new("deals.csv")},
		{Line: new(7), Reason: "-1 is not a size"},
		{Reason: "a new tag", File: new("companies.csv"), Value: new("vip")},
	}}
	var buf bytes.Buffer
	if err := writeImportProblems(&buf, rep, textlang.English); err != nil {
		t.Fatal(err)
	}
	want := "file,row,column,problem,value\n" +
		"deals.csv,3,Value,not a number,\"'=HYPERLINK(\"\"x\"\")\"\n" +
		",7,,'-1 is not a size,\n" +
		"companies.csv,,,a new tag,vip\n"
	if buf.String() != want {
		t.Fatalf("csv = %q, want %q", buf.String(), want)
	}
}

func TestProblemsCSVWritesTheCatalogEnglishForACodedIssue(t *testing.T) {
	code := crmcontracts.ImportReasonCode("link_not_found")
	params := map[string]string{"ref": "CO-7"}
	rep := crmcontracts.ImportRunReport{Issues: []crmcontracts.ImportRowIssue{
		{Line: new(2), Reason: "no company with \"CO-7\" in this upload", ReasonCode: &code, Params: &params},
	}}
	var buf bytes.Buffer
	if err := writeImportProblems(&buf, rep, textlang.English); err != nil {
		t.Fatal(err)
	}
	want := "file,row,column,problem,value\n" +
		",2,,\"No company with “CO-7” in this upload, in an earlier import, or in Margince.\",\n"
	if buf.String() != want {
		t.Fatalf("csv = %q, want %q", buf.String(), want)
	}
}
