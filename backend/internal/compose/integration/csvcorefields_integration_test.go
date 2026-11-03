// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type contactPhoneDTO struct {
	Phone     string `json:"phone"`
	PhoneType string `json:"phone_type"`
	IsPrimary bool   `json:"is_primary"`
	Position  int    `json:"position"`
}

type contactCoreDTO struct {
	Phones []contactPhoneDTO `json:"phones"`
	Social map[string]any    `json:"social"`
}

func contactCore(t *testing.T, e *apptest.AppEnv, id string) contactCoreDTO {
	t.Helper()
	var contact contactCoreDTO
	if status := e.Call(t, http.MethodGet, "/v1/contacts/"+id, nil, nil, &contact); status != http.StatusOK {
		t.Fatalf("GET /v1/contacts/%s → %d, want 200", id, status)
	}
	return contact
}

// addPhoneByHand adds a mobile number through the contact's own edit, keeping
// the numbers it holds.
func addPhoneByHand(t *testing.T, e *apptest.AppEnv, id, phone string) {
	t.Helper()
	phones := contactCore(t, e, id).Phones
	phones = append(phones, contactPhoneDTO{Phone: phone, PhoneType: "mobile", Position: len(phones) + 1})
	if status := e.Call(t, http.MethodPatch, "/v1/contacts/"+id, AnyMap{"phones": phones}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH /v1/contacts/%s phones → %d, want 200", id, status)
	}
}

func contactPhones(t *testing.T, e *apptest.AppEnv, id string) []string {
	t.Helper()
	var numbers []string
	for _, held := range contactCore(t, e, id).Phones {
		numbers = append(numbers, held.Phone)
	}
	return numbers
}

func TestAContactsPhoneAndLinkedInLandAndAReImportKeepsTheOthers(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "contact", "email,name,phone,linkedin\nmy@core.example,My,+49 30 1234567,https://www.linkedin.com/in/my-core\n")
	run, _ := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"email": "email", "name": "full_name", "phone": "phone", "linkedin": "linkedin_url"})
	approveAndRead(t, e, run.ID)
	id := contactIDByEmail(t, e, "my@core.example")
	if got := contactCore(t, e, id).Social["linkedin"]; got != "https://www.linkedin.com/in/my-core" {
		t.Fatalf("social.linkedin = %v, want the file's profile", got)
	}
	addPhoneByHand(t, e, id, "+84 28 1234 5678")
	again, _ := uploadCSV(t, e, "contact", "email,name,phone\nmy@core.example,My,+49 30 7654321\n")
	second, _ := createRunWithMapping(t, e, "contact", again.SourceRef, map[string]string{"email": "email", "name": "full_name", "phone": "phone"})
	approveAndRead(t, e, second.ID)
	phones := contactPhones(t, e, id)
	if !slices.Contains(phones, "+49307654321") || !slices.Contains(phones, "+842812345678") {
		t.Fatalf("phones = %v, want the file's new number and the one added by hand, both kept", phones)
	}
	if got := contactCore(t, e, id).Social["linkedin"]; got != "https://www.linkedin.com/in/my-core" {
		t.Fatalf("social.linkedin = %v after a file with no LinkedIn column, want it kept", got)
	}
}

func TestAPhoneWithoutACountryCodeIsAProblemNotAFailedRun(t *testing.T) {
	e := setupImportApp(t)
	profile, _ := uploadCSV(t, e, "contact", "email,name,phone\nno@cc.example,No,030 1234567\nok@cc.example,Ok,+49 30 1\n")
	run, _ := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"email": "email", "name": "full_name", "phone": "phone"})
	var report importReportDTO
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report)
	if !hasIssue(report, "invalid_value", "030 1234567") {
		t.Fatalf("report = %+v, want the local number named as an invalid value", report)
	}
}

func TestACompanyAndALeadCarryTheirLinkedInURL(t *testing.T) {
	e := setupImportApp(t)
	companies, _ := uploadCSV(t, e, "company", "Name,LinkedIn\nVoltaq GmbH,https://www.linkedin.com/company/voltaq/about\n")
	run, _ := createRunWithMapping(t, e, "company", companies.SourceRef, map[string]string{"Name": "display_name", "LinkedIn": "linkedin_url"})
	approveAndRead(t, e, run.ID)
	if got := companyField(t, e, "Voltaq GmbH", "linkedin_url"); got != "https://www.linkedin.com/company/voltaq" {
		t.Fatalf("company linkedin_url = %q, want the canonical company URL", got)
	}

	leads, _ := uploadCSV(t, e, "lead", "Email,Name,LinkedIn\nlee@core.example,Lee,linkedin.com/in/lee/\n")
	leadRun, _ := createRunWithMapping(t, e, "lead", leads.SourceRef, map[string]string{"Email": "email", "Name": "full_name", "LinkedIn": "linkedin_url"})
	approveAndRead(t, e, leadRun.ID)
	if got := recordField(t, e, "/v1/leads", "email", "lee@core.example", "linkedin_url"); got != "https://www.linkedin.com/in/lee" {
		t.Fatalf("lead linkedin_url = %q, want the normalized profile", got)
	}

	edited, _ := uploadCSV(t, e, "lead", "Email,Name,LinkedIn\nlee@core.example,Lee,https://www.linkedin.com/in/lee-new\n")
	editRun, _ := createRunWithMapping(t, e, "lead", edited.SourceRef, map[string]string{"Email": "email", "Name": "full_name", "LinkedIn": "linkedin_url"})
	approveAndRead(t, e, editRun.ID)
	if got := recordField(t, e, "/v1/leads", "email", "lee@core.example", "linkedin_url"); got != "https://www.linkedin.com/in/lee-new" {
		t.Fatalf("lead linkedin_url after the edited file = %q, want the new profile", got)
	}
}

func TestACompanyWhoseLinkedInURLIsHeldIsSkippedInThePreviewAndTheCommit(t *testing.T) {
	e := setupImportApp(t)
	var held struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, http.MethodPost, "/v1/companies", AnyMap{"display_name": "Held AG", "source": "manual"}, nil, &held); status != http.StatusCreated {
		t.Fatalf("POST /v1/companies → %d, want 201", status)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/companies/"+held.ID,
		AnyMap{"linkedin_url": "https://www.linkedin.com/company/held"}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH /v1/companies/%s → %d, want 200", held.ID, status)
	}
	profile, _ := uploadCSV(t, e, "company", "Name,LinkedIn\nOther Held GmbH,https://www.linkedin.com/company/HELD/\n")
	run, _ := createRunWithMapping(t, e, "company", profile.SourceRef, map[string]string{"Name": "display_name", "LinkedIn": "linkedin_url"})
	preview := readImportReport(t, e, run.ID)
	if preview.Disposition.Skipped != 1 {
		t.Fatalf("preview disposition = %+v, want the row skipped", preview.Disposition)
	}
	approveAndRead(t, e, run.ID)
	if report := readImportReport(t, e, run.ID); report.Disposition.Skipped != 1 || report.Disposition.Created != 0 {
		t.Fatalf("commit disposition = %+v, want the row skipped, as previewed", report.Disposition)
	}
}
