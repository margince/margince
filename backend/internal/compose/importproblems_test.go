// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestProblemsCSVDefusesAFormulaInAValue(t *testing.T) {
	rep := crmcontracts.ImportRunReport{Issues: []crmcontracts.ImportRowIssue{
		{Line: 3, Reason: "not a number", Column: new("Value"), Value: new(`=HYPERLINK("x")`), File: new("deals.csv")},
		{Line: 7, Reason: "-1 is not a size"},
	}}
	var buf bytes.Buffer
	if err := writeImportProblems(&buf, rep); err != nil {
		t.Fatal(err)
	}
	want := "file,row,column,problem,value\n" +
		"deals.csv,3,Value,not a number,\"'=HYPERLINK(\"\"x\"\")\"\n" +
		",7,,'-1 is not a size,\n"
	if buf.String() != want {
		t.Fatalf("csv = %q, want %q", buf.String(), want)
	}
}
