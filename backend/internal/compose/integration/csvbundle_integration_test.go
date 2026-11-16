// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/importjobtest"
)

func TestAZipUploadProfilesEveryFileAndListsWhatItIgnores(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{
		"export/companies.csv": "\ufeffref,name,domain\nCO-1,Acme,acme.example\n",
		"export/contacts.csv":  "ref;email;company_ref\nP-1;jo@acme.example;CO-1\n",
		"products.csv":         "Product\nWidget\n",
		"__MACOSX/._x":         "junk",
	})
	profile := uploadZip(t, e, "HubSpot export (2).zip", body)
	if profile.FileName != "HubSpot export (2).zip" || profile.SuggestedSourceName != "HubSpot export -2-" || len(profile.Parts) != 2 {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.Parts[0].Object != "company" || profile.Parts[1].Object != "contact" {
		t.Errorf("parts = %+v, want companies before contacts (import order), whatever order the ZIP holds them in", profile.Parts)
	}
	byFile := map[string]importPartProfileDTO{}
	for _, p := range profile.Parts {
		byFile[p.File] = p
		if p.RowsProfiled != 1 {
			t.Errorf("%s profiled %d rows, want 1", p.File, p.RowsProfiled)
		}
	}
	if got := byFile["export/companies.csv"].SuggestedMappings["company"]["domain"]; got != "domain" {
		t.Errorf("company suggestion for domain = %q", got)
	}
	if len(profile.Ignored) != 1 || profile.Ignored[0].File != "products.csv" {
		t.Errorf("ignored = %+v, want products.csv alone", profile.Ignored)
	}
}

func TestAZipWithAFileThatIsNotUTF8IsRefusedNamingTheFile(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{"companies.csv": "name\nM\xfcller GmbH\n"})
	raw, status := postZipFile(t, e, "x.zip", body)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "companies.csv") {
		t.Fatalf("status %d body %s, want 422 naming companies.csv", status, raw)
	}
}

func TestAReaderWhoNamesTheTypeOfAnUntypedCSVLeavesNoChoiceByNothing(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	raw, status := importjobtest.PostCSVFile(t, e, "", prospectCSV)
	var profile struct {
		RunID          string `json:"run_id"`
		Object         string `json:"object"`
		ObjectChosenBy string `json:"object_chosen_by"`
	}
	if err := json.Unmarshal(raw, &profile); err != nil || status != http.StatusOK || profile.ObjectChosenBy != "none" {
		t.Fatalf("status %d body %s (%v), want a lead nothing chose", status, raw, err)
	}
	if s := e.Call(t, http.MethodPatch, "/v1/imports/"+profile.RunID, AnyMap{
		"version": 1, "object": profile.Object,
	}, nil, nil); s != http.StatusOK {
		t.Fatalf("PATCH object → %d, want 200", s)
	}
	var run struct {
		Profile map[string]any `json:"profile"`
	}
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+profile.RunID, nil, nil, &run); s != http.StatusOK ||
		run.Profile == nil || run.Profile["object_chosen_by"] != nil {
		t.Fatalf("run → %d %+v, want its profile to carry the reader's choice and no object_chosen_by", s, run)
	}
}

func TestACSVUploadWithoutATypeIsTypedByItsColumns(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	for _, tc := range []struct {
		name, csv, object, chosenBy string
	}{
		{"a contact's own columns", "Email,First Name,Last Name\nada@x.test,Ada,Lovelace\n", "contact", "columns"},
		{"columns several types share", prospectCSV, "lead", "none"},
		{"a deal's own columns", "Pipeline,Stage,Amount\nSales,Won,100\n", "deal", "columns"},
		{"a common header beside a company's own column", "Name,Domain\nAcme,acme.example\n", "company", "columns"},
		{"two spellings of one column count for neither", "First Name,first_name,Status\nAda,Ada,new\n", "lead", "columns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, status := importjobtest.PostCSVFile(t, e, "", tc.csv)
			var profile struct {
				RunID          string `json:"run_id"`
				Object         string `json:"object"`
				ObjectChosenBy string `json:"object_chosen_by"`
			}
			if err := json.Unmarshal(raw, &profile); err != nil || status != http.StatusOK {
				t.Fatalf("status %d body %s (%v), want 200", status, raw, err)
			}
			if profile.Object != tc.object || profile.ObjectChosenBy != tc.chosenBy {
				t.Fatalf("profile = %+v, want %s chosen by %s", profile, tc.object, tc.chosenBy)
			}
			var run struct {
				Profile struct {
					ObjectChosenBy string `json:"object_chosen_by"`
				} `json:"profile"`
			}
			if s := e.Call(t, http.MethodGet, "/v1/imports/"+profile.RunID, nil, nil, &run); s != http.StatusOK ||
				run.Profile.ObjectChosenBy != tc.chosenBy {
				t.Fatalf("run → %d %+v, want its profile to keep %s", s, run, tc.chosenBy)
			}
		})
	}
	if status, errs := uploadRefusal(t, e, "widget", prospectCSV); status != http.StatusUnprocessableEntity || len(errs) == 0 || errs[0].Field != "object" {
		t.Fatalf("status %d errs %+v, want 422 on an object no import takes", status, errs)
	}
	raw, status := importjobtest.PostCSVFile(t, e, "lead", "Email,First Name,Last Name\nbo@x.test,Bo,Ek\n")
	if status != http.StatusOK || strings.Contains(string(raw), "object_chosen_by") || !strings.Contains(string(raw), `"object":"lead"`) {
		t.Fatalf("status %d body %s, want a named type kept and no choice reported", status, raw)
	}
}

func TestAZipHoldingOneTypeTwiceIsRefusedAsABundle(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{"companies.csv": "name\nAcme\n", "a/company.csv": "name\nBeta\n"})
	raw, status := postZipFile(t, e, "x.zip", body)
	errs := fieldRefusalsOf(t, raw)
	if status != http.StatusUnprocessableEntity || len(errs) != 1 || errs[0].Code != "bundle_refused" {
		t.Fatalf("status %d body %s, want 422 bundle_refused", status, raw)
	}
	files := map[string]bool{errs[0].Params["file"]: true, errs[0].Params["other_file"]: true}
	if errs[0].Params["reason"] != "two_files_one_type" || errs[0].Params["object"] != "company" ||
		!files["companies.csv"] || !files["a/company.csv"] {
		t.Fatalf("params = %v, want the reason, the type and both files", errs[0].Params)
	}
}

func TestAnEmptyZipAndARaggedRowAreRefusedWithWhatTheirSentencesName(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	raw, status := postZipFile(t, e, "empty.zip", zipBytes(t, nil))
	if errs := fieldRefusalsOf(t, raw); status != http.StatusUnprocessableEntity || len(errs) != 1 ||
		errs[0].Code != "bundle_refused" || errs[0].Params["reason"] != "empty" {
		t.Fatalf("status %d body %s, want 422 bundle_refused for an empty ZIP", status, raw)
	}
	status, errs := uploadRefusal(t, e, "contact", "Email,Full Name\nada@x.test\n")
	if status != http.StatusUnprocessableEntity || len(errs) != 1 || errs[0].Code != "unreadable" ||
		errs[0].Params["line"] != "2" || errs[0].Params["reason"] != "field_count" {
		t.Fatalf("status %d errs %+v, want 422 unreadable naming line 2 and its cause", status, errs)
	}
}

// A client that tells a ZIP by its name sends `object` for a ZIP renamed to
// .csv. The server knows the ZIP by its content and reads each part's type from
// its file name, as the contract states.
func TestAZipSentWithAnObjectIsProfiledByItsFileNames(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{"companies.csv": "ref,name\nCO-1,Acme\n"})
	raw, status := importjobtest.PostCSVFile(t, e, "lead", string(body))
	if status != http.StatusOK {
		t.Fatalf("status %d body %s, want 200", status, raw)
	}
	var profile importBundleProfileDTO
	mustJSON(t, raw, &profile)
	if len(profile.Parts) != 1 || profile.Parts[0].Object != "company" {
		t.Fatalf("parts = %+v, want the one companies.csv part as company, not lead", profile.Parts)
	}
}

// A workbook, or a ZIP of files no type takes, opens no draft. The upload is
// refused before anything is stored, and says what to send instead.
func TestAZipWithNoImportableFileIsRefusedAndLeavesNoDraft(t *testing.T) {
	t.Parallel()
	e := setupImportApp(t)
	for name, c := range map[string]struct {
		files  map[string]string
		reason string
	}{
		"Pipeline.csv": {map[string]string{"[Content_Types].xml": "<Types/>", "xl/workbook.xml": "<workbook/>"}, "spreadsheet"},
		"other.zip":    {map[string]string{"products.csv": "Product\nWidget\n"}, "no_importable_file"},
	} {
		raw, status := postZipFile(t, e, name, zipBytes(t, c.files))
		errs := fieldRefusalsOf(t, raw)
		if status != http.StatusUnprocessableEntity || len(errs) != 1 || errs[0].Code != "bundle_refused" ||
			errs[0].Params["reason"] != c.reason {
			t.Errorf("%s: status %d body %s, want 422 bundle_refused for %s", name, status, raw, c.reason)
		}
	}
	if runs := importRuns(t, e); len(runs) != 0 {
		t.Fatalf("runs = %+v, want no draft left by a refused upload", runs)
	}
}
