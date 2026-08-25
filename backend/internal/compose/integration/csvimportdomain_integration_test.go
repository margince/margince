// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company file's DOMAIN column, end to end.
//
// Split from csvimport_integration_test.go on the concept: a domain is the one
// company field held as a child collection with estate-wide uniqueness behind
// it, so every promise here — normalization, convergence, refusal, and not
// archiving what the file never mentioned — is one the flat columns never make.

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// A company file's domain column lands, and a re-import of it converges.
//
// The convergence half is the one that catches the defect this shape invites: a
// company holds its domains as child rows, so a comparison reading the record's
// flat fields finds no `domain`, calls every row changed, and reports an update
// forever. It also proves the spelling is normalized — the file says
// "https://www.northwind.example/" and the store holds "northwind.example".
func TestCSVImportLandsDomainsAndConverges(t *testing.T) {
	e := setupImportApp(t)

	const companies = "Company,Website\n" +
		"Northwind,https://www.northwind.example/\n" +
		"Contoso,contoso.example\n"
	profile, status := uploadCSV(t, e, "organization", companies)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	if !slices.Contains(profile.Targets, "domain") {
		t.Fatalf("targets = %v, want `domain` offered to a company file", profile.Targets)
	}
	mapping := map[string]string{"Company": "display_name", "Website": "domain"}

	run, runStatus := createRunWithMapping(t, e, "organization", profile.SourceRef, mapping)
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}
	var approved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}

	var orgs struct {
		Data []struct {
			DisplayName string `json:"display_name"`
			Domains     []struct {
				Domain    string `json:"domain"`
				IsPrimary bool   `json:"is_primary"`
			} `json:"domains"`
		} `json:"data"`
	}
	if s := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); s != http.StatusOK {
		t.Fatalf("GET /v1/organizations → %d, want 200", s)
	}
	found := map[string]string{}
	for _, org := range orgs.Data {
		if len(org.Domains) > 0 {
			found[org.DisplayName] = org.Domains[0].Domain
		}
	}
	if found["Northwind"] != "northwind.example" {
		t.Fatalf("Northwind domain = %q, want the URL reduced to its host", found["Northwind"])
	}
	if found["Contoso"] != "contoso.example" {
		t.Fatalf("Contoso domain = %q, want it landed", found["Contoso"])
	}

	// The identical file again: nothing changed, so nothing is written. A domain
	// the comparison could not read would report two updates instead.
	second, secondStatus := uploadCSV(t, e, "organization", companies)
	if secondStatus != http.StatusOK {
		t.Fatalf("second upload → %d, want 200", secondStatus)
	}
	rerun, rerunStatus := createRunWithMapping(t, e, "organization", second.SourceRef, mapping)
	if rerunStatus != http.StatusAccepted {
		t.Fatalf("create re-run → %d, want 202", rerunStatus)
	}
	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+rerun.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("re-report → %d, want 200", s)
	}
	if report.Disposition.Created != 0 || report.Disposition.Updated != 0 {
		t.Fatalf("re-run = %+v, want nothing created and nothing updated", report.Disposition)
	}
}

// A domain another company already holds refuses the row, and says so.
//
// A domain names ONE company across the estate — that is what makes it a real
// key, and what makes it the thing dedupe should match on. The row is a skip
// with a reason; the rest of the file lands.
func TestCSVImportRefusesADomainAnotherCompanyHolds(t *testing.T) {
	e := setupImportApp(t)
	if status := e.Call(t, http.MethodPost, "/v1/organizations",
		map[string]any{"display_name": "Northwind Traders", "domains": []map[string]any{
			{"domain": "northwind.example", "is_primary": true},
		}}, nil, nil); status != http.StatusCreated {
		t.Fatalf("creating the incumbent → %d, want 201", status)
	}

	const companies = "Company,Website\n" +
		"Northwind Copy,northwind.example\n" +
		"Contoso,contoso.example\n"
	profile, status := uploadCSV(t, e, "organization", companies)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, runStatus := createRunWithMapping(t, e, "organization", profile.SourceRef,
		map[string]string{"Company": "display_name", "Website": "domain"})
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}
	// BEFORE approval: the preview must already say the row will be skipped.
	// Reading only the committed report would pass while the preview promised a
	// create — the disagreement this pipeline exists to prevent.
	var preview importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &preview); s != http.StatusOK {
		t.Fatalf("preview → %d, want 200", s)
	}
	if preview.Disposition.Created != 1 || preview.Disposition.Skipped != 1 {
		t.Fatalf("preview = %+v, want the taken domain predicted as a skip", preview.Disposition)
	}

	var approved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}

	var report importReportDTO
	if s := e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report); s != http.StatusOK {
		t.Fatalf("report → %d, want 200", s)
	}
	if report.Disposition.Created != 1 || report.Disposition.Skipped != 1 {
		t.Fatalf("commit = %+v, want the same counts the preview promised", report.Disposition)
	}
	var named bool
	for _, issue := range report.Issues {
		if strings.Contains(issue.Reason, "already held by another company") {
			named = true
		}
	}
	if !named {
		t.Fatalf("issues = %+v, want the skip to say the domain is already held", report.Issues)
	}
}

// A file naming one domain does not archive the company's other domains.
//
// The store's domain set is a REPLACE-SET: whatever it is given becomes the
// whole live set. That is right for a form submitting every domain a company
// has and wrong for a spreadsheet carrying one Website column, so the import
// merges rather than replaces — a file whose columns are whatever the customer
// exported may not delete what it never mentioned.
//
// The company is IMPORTED first rather than created by hand, because only then
// does the identity map recognise the second file as a correction of the first.
// A company created another way is invisible to it, and a second file naming the
// same company creates a twin — which is the by-name matching this whole target
// exists to replace.
func TestCSVImportOfOneDomainKeepsTheCompanysOthers(t *testing.T) {
	e := setupImportApp(t)
	mapping := map[string]string{"Company": "display_name", "Website": "domain"}

	const first = "Company,Website\nFabrikam,fabrikam.example\n"
	profile, status := uploadCSV(t, e, "organization", first)
	if status != http.StatusOK {
		t.Fatalf("upload → %d, want 200", status)
	}
	run, runStatus := createRunWithMapping(t, e, "organization", profile.SourceRef, mapping)
	if runStatus != http.StatusAccepted {
		t.Fatalf("create run → %d, want 202", runStatus)
	}
	var approved importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run.ID+"/approve", nil, nil, &approved); s != http.StatusAccepted {
		t.Fatalf("approve → %d, want 202", s)
	}

	// A human adds a second domain the spreadsheet has no column for.
	var orgs struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if s := e.Call(t, http.MethodGet, "/v1/organizations?limit=100", nil, nil, &orgs); s != http.StatusOK {
		t.Fatalf("GET /v1/organizations → %d, want 200", s)
	}
	var orgID string
	for _, org := range orgs.Data {
		if org.DisplayName == "Fabrikam" {
			orgID = org.ID
		}
	}
	if orgID == "" {
		t.Fatal("the imported company is not in the list")
	}
	if s := e.Call(t, http.MethodPatch, "/v1/organizations/"+orgID,
		map[string]any{"domains": []map[string]any{
			{"domain": "fabrikam.example", "is_primary": true},
			{"domain": "fabrikam.co.example", "is_primary": false},
		}}, nil, nil); s != http.StatusOK {
		t.Fatalf("adding the second domain → %d, want 200", s)
	}

	// The corrected file names only the primary. The hand-added one must survive.
	const second = "Company,Website\nFabrikam,fabrikam-group.example\n"
	profile2, status2 := uploadCSV(t, e, "organization", second)
	if status2 != http.StatusOK {
		t.Fatalf("second upload → %d, want 200", status2)
	}
	run2, run2Status := createRunWithMapping(t, e, "organization", profile2.SourceRef, mapping)
	if run2Status != http.StatusAccepted {
		t.Fatalf("create second run → %d, want 202", run2Status)
	}
	var approved2 importRunDTO
	if s := e.Call(t, http.MethodPost, "/v1/imports/"+run2.ID+"/approve", nil, nil, &approved2); s != http.StatusAccepted {
		t.Fatalf("approve second → %d, want 202", s)
	}

	var after struct {
		Domains []struct {
			Domain    string `json:"domain"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"domains"`
	}
	if s := e.Call(t, http.MethodGet, "/v1/organizations/"+orgID, nil, nil, &after); s != http.StatusOK {
		t.Fatalf("re-reading the company → %d, want 200", s)
	}
	var held []string
	var primary string
	for _, d := range after.Domains {
		held = append(held, d.Domain)
		if d.IsPrimary {
			primary = d.Domain
		}
	}
	if !slices.Contains(held, "fabrikam.co.example") {
		t.Fatalf("domains = %v, want the one the file never mentioned still held", held)
	}
	if primary != "fabrikam-group.example" {
		t.Fatalf("primary = %q, want the file's domain", primary)
	}
}
