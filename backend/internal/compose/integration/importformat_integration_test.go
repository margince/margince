// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/csv"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func readImportFormat(t *testing.T, e *apptest.AppEnv, query string) crmcontracts.ImportFormat {
	t.Helper()
	var f crmcontracts.ImportFormat
	if status := e.Call(t, http.MethodGet, "/v1/imports/format"+query, nil, nil, &f); status != http.StatusOK {
		t.Fatalf("GET /v1/imports/format%s → %d, want 200", query, status)
	}
	return f
}

func formatColumn(f crmcontracts.ImportFormat, file, name string) *crmcontracts.ImportFormatColumn {
	for _, part := range f.Files {
		if part.File != file {
			continue
		}
		for i := range part.Columns {
			if part.Columns[i].Name == name {
				return &part.Columns[i]
			}
		}
	}
	return nil
}

func TestTheFormatListsACustomFieldByItsKeyWithItsLabel(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	column := apptest.CreateCustomField(t, e, AnyMap{"object": "company", "label": "Tier", "type": "picklist", "options": []string{"A", "B"}, "source": "manual"})
	german := formatColumn(readImportFormat(t, e, "?lang=de"), "companies.csv", column)
	english := formatColumn(readImportFormat(t, e, "?lang=en"), "companies.csv", column)
	if german == nil || english == nil {
		t.Fatalf("companies.csv has no %s column", column)
	}
	if german.Label == nil || *german.Label != "Tier" || !german.Custom || german.Options == nil || !slices.Equal(*german.Options, []string{"A", "B"}) {
		t.Fatalf("%s = %+v, want label Tier, custom, options A and B", column, german)
	}
	if german.Description == "" || german.Description == english.Description {
		t.Errorf("German description %q, English %q; want a German one", german.Description, english.Description)
	}
	if status := e.Call(t, http.MethodGet, "/v1/imports/format?lang=fr", nil, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("lang=fr → %d, want 422", status)
	}
}

func TestTheTemplateHeadersAreTheFormatsColumns(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	apptest.CreateCustomField(t, e, AnyMap{"object": "contact", "label": "Segment, internal", "type": "text", "source": "manual"})
	status, header, archive := getRaw(t, e, "/v1/imports/template", nil)
	if status != http.StatusOK || header.Get("Content-Type") != "application/zip" ||
		!strings.Contains(header.Get("Content-Disposition"), "margince-import.zip") {
		t.Fatalf("GET /v1/imports/template → %d %v", status, header)
	}
	f := readImportFormat(t, e, "")
	if len(f.Files) == 0 {
		t.Fatal("the format lists no files")
	}
	for _, part := range f.Files {
		want := make([]string, 0, len(part.Columns))
		for _, col := range part.Columns {
			if col.Label != nil {
				want = append(want, *col.Label)
				continue
			}
			want = append(want, col.Name)
		}
		got, err := csv.NewReader(strings.NewReader(zipFile(t, archive, part.File))).Read()
		if err != nil {
			t.Fatalf("reading %s's header: %v", part.File, err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s header %v, want the format's columns %v", part.File, got, want)
		}
	}
	if readme := zipFile(t, archive, "README.txt"); !strings.Contains(readme, "Segment, internal") {
		t.Errorf("README lacks the custom column:\n%s", readme)
	}
}

func TestProblemsCSVListsTheReportsIssues(t *testing.T) {
	e := setupCustomFieldImportApp(t)
	column := apptest.CreateCustomField(t, e, AnyMap{"object": "company", "label": "Tier", "type": "picklist", "options": []string{"Gold"}, "source": "manual"})
	profile, _ := uploadCSV(t, e, "company", "Name,Tier\nGood GmbH,Gold\nBad GmbH,Bronze\n")
	run, status := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "Tier": column})
	if status != http.StatusAccepted {
		t.Fatalf("stage → %d, want 202", status)
	}
	status, header, body := getRaw(t, e, "/v1/imports/"+run.ID+"/problems.csv", nil)
	if status != http.StatusOK || !strings.HasPrefix(header.Get("Content-Type"), "text/csv") {
		t.Fatalf("GET problems.csv → %d %v: %s", status, header, body)
	}
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatalf("reading problems.csv: %v", err)
	}
	if len(rows) != 2 || !slices.Equal(rows[0], []string{"file", "row", "column", "problem", "value"}) ||
		rows[1][1] != "3" || rows[1][2] != "Tier" || rows[1][4] != "Bronze" {
		t.Fatalf("problems.csv = %v, want the header and the Bronze row on line 3", rows)
	}
	missing, _, _ := getRaw(t, e, "/v1/imports/00000000-0000-0000-0000-000000000001/problems.csv", nil)
	if missing != http.StatusNotFound {
		t.Errorf("an unknown run → %d, want 404", missing)
	}
}
