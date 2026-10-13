// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

func TestAZipUploadProfilesEveryFileAndListsWhatItIgnores(t *testing.T) {
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
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{"companies.csv": "name\nM\xfcller GmbH\n"})
	raw, status := postZipFile(t, e, "x.zip", body)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "companies.csv") {
		t.Fatalf("status %d body %s, want 422 naming companies.csv", status, raw)
	}
}

func TestACSVUploadWithoutATypeIsStillRefused(t *testing.T) {
	e := setupImportApp(t)
	if status, errs := uploadRefusal(t, e, "", prospectCSV); status != http.StatusUnprocessableEntity || len(errs) == 0 || errs[0].Field != "object" {
		t.Fatalf("status %d errs %+v, want 422 on object", status, errs)
	}
}

func TestAZipHoldingOneTypeTwiceIsRefusedAsABundle(t *testing.T) {
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{"companies.csv": "name\nAcme\n", "a/company.csv": "name\nBeta\n"})
	raw, status := postZipFile(t, e, "x.zip", body)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(raw), "bundle_refused") {
		t.Fatalf("status %d body %s, want 422 bundle_refused", status, raw)
	}
}

// A client that tells a ZIP by its name sends `object` for a ZIP renamed to
// .csv. The server knows the ZIP by its content and reads each part's type from
// its file name, as the contract states.
func TestAZipSentWithAnObjectIsProfiledByItsFileNames(t *testing.T) {
	e := setupImportApp(t)
	body := zipBytes(t, map[string]string{"companies.csv": "ref,name\nCO-1,Acme\n"})
	raw, status := postCSVFile(t, e, "lead", string(body))
	if status != http.StatusOK {
		t.Fatalf("status %d body %s, want 200", status, raw)
	}
	var profile importBundleProfileDTO
	mustJSON(t, raw, &profile)
	if len(profile.Parts) != 1 || profile.Parts[0].Object != "company" {
		t.Fatalf("parts = %+v, want the one companies.csv part as company, not lead", profile.Parts)
	}
}
