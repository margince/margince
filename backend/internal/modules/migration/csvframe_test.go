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

func TestAnExportedFileReadsBackAsTheValuesItWasWrittenFrom(t *testing.T) {
	body := "Name,Industry,Description,Phone\n" +
		"'=Name,'-Retail,\"'@mention, \"\"quoted\"\" and, comma\",'+84901234567\n"
	want := map[string]string{
		"name": "=Name", "industry": "-Retail",
		"description": `@mention, "quoted" and, comma`, "phone": "+84901234567",
	}

	p, err := ProfileCSV(strings.NewReader(body), ProfileRowLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Columns {
		if got := c.Samples[0]; got != want[strings.ToLower(c.Header)] {
			t.Errorf("profile sample for %s = %q, want %q", c.Header, got, want[strings.ToLower(c.Header)])
		}
	}

	src := NewCSVSource(seedCSV(t, body), testCSVKey, ObjectCompany,
		map[string]string{"Name": "name", "Industry": "industry", "Description": "description", "Phone": "phone"}, "Name")
	rows, err := src.Rows(context.Background(), ObjectCompany, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one", rows)
	}
	for field, value := range want {
		if got := rows[0].Fields[field]; got != value {
			t.Errorf("walked %s = %q, want %q", field, got, value)
		}
	}
}
