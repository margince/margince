// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"strings"
	"testing"
)

func TestTheProfileDropsTheByteOrderMarkExcelWrites(t *testing.T) {
	p, err := ProfileCSV(strings.NewReader("\xEF\xBB\xBFEmail,Full Name\nada@x.test,Ada\n"), ProfileRowLimit)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Columns[0].Header; got != "Email" {
		t.Fatalf("first header = %q, want %q: the mark leaked into the column name", got, "Email")
	}
}

func TestASemicolonFileIsReadAsSemicolonSeparated(t *testing.T) {
	p, err := ProfileCSV(strings.NewReader("Email;Full Name;Title\nada@x.test;Ada;Analyst, Senior\n"), ProfileRowLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Columns) != 3 || p.Columns[2].Samples[0] != "Analyst, Senior" {
		t.Fatalf("columns = %+v, want three with the comma kept inside the title", p.Columns)
	}
}

func TestADelimiterInsideQuotesDoesNotDecideTheFraming(t *testing.T) {
	p, err := ProfileCSV(strings.NewReader("\"Name; legal\",Note\nAcme,x\n"), ProfileRowLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Columns) != 2 || p.Columns[0].Header != "Name; legal" {
		t.Fatalf("columns = %+v, want a comma file whose first header holds a semicolon", p.Columns)
	}
}

func TestTheWalkFramesAFileExactlyAsTheProfileDid(t *testing.T) {
	body := "\xEF\xBB\xBFEmail;Full Name\nada@x.test;Ada\n"
	src := NewCSVSource(seedCSV(t, body), testCSVKey, ObjectLead,
		map[string]string{"Email": "email", "Full Name": "full_name"}, "Email")
	rows, err := src.Rows(context.Background(), ObjectLead, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Fields["full_name"] != "Ada" || rows[0].Line != 2 {
		t.Fatalf("rows = %+v, want one row from line 2 with its name", rows)
	}
}
