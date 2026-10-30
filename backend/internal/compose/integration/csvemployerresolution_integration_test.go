// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
)

func TestCSVEmployerConflictSkipsOnlyTheRefusedRow(t *testing.T) {
	e := setupImportApp(t)
	for _, company := range []map[string]any{
		{"source": "manual", "display_name": "Named Works"},
		{"source": "manual", "display_name": "Domain Works", "domains": []map[string]any{{"domain": "domain-work.test", "is_primary": true}}},
	} {
		if s := e.Call(t, http.MethodPost, "/v1/companies", company, nil, nil); s != http.StatusCreated {
			t.Fatalf("company: %d", s)
		}
	}
	profile, _ := uploadCSV(t, e, "contact", "Email,Full Name,Company\nbuyer@domain-work.test,Conflicting Buyer,Named Works\nother@fresh-work.test,Other Buyer,Fresh Works\n")
	run, status := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"Email": "email", "Full Name": "full_name", "Company": "company_name"})
	if status != http.StatusAccepted {
		t.Fatalf("run: %d", status)
	}
	if s := importCall(t, e, run.ID, "approve", nil, nil); s != http.StatusAccepted {
		t.Fatalf("approve: %d", s)
	}
	if got := readRun(t, e, run.ID); got.Status != "complete" {
		t.Fatalf("run: %+v", got)
	}
	if n := importedContactCount(t, e); n != 1 {
		t.Fatalf("contacts: %d, want only the unambiguous row", n)
	}
}

func TestCSVImplicitEmployerIsRecordedForUndo(t *testing.T) {
	e := setupImportApp(t)
	before := len(companyRows(t, e))
	profile, _ := uploadCSV(t, e, "contact", "Email,Full Name,Company\nbuyer@undo-work.test,Dana Buyer,Undo Works\n")
	run, status := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"Email": "email", "Full Name": "full_name", "Company": "company_name"})
	if status != http.StatusAccepted {
		t.Fatalf("run: %d", status)
	}
	if s := importCall(t, e, run.ID, "approve", nil, nil); s != http.StatusAccepted {
		t.Fatalf("approve: %d", s)
	}
	if got := len(companyRows(t, e)); got != before+1 {
		t.Fatalf("companies: %d", got)
	}
	if s := importCall(t, e, run.ID, "undo", nil, nil); s != http.StatusAccepted {
		t.Fatalf("undo: %d", s)
	}
	if got := len(companyRows(t, e)); got != before {
		t.Fatalf("company survived undo: %d", got)
	}
}

func TestCSVContactUsesConfiguredConsumerDomains(t *testing.T) {
	e := setupImportApp(t)
	if s := e.Call(t, http.MethodPost, "/v1/capture/consumer-mail-domains", map[string]string{"domain": "mail-provider.test", "kind": "extra"}, nil, nil); s != http.StatusCreated {
		t.Fatalf("domain: %d", s)
	}
	if s := e.Call(t, http.MethodPost, "/v1/companies", map[string]any{"source": "manual", "display_name": "Mail Provider", "domains": []map[string]any{{"domain": "mail-provider.test", "is_primary": true}}}, nil, nil); s != http.StatusCreated {
		t.Fatalf("company: %d", s)
	}
	profile, _ := uploadCSV(t, e, "contact", "Email,Full Name\nbuyer@mail-provider.test,Dana Buyer\n")
	run, status := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"Email": "email", "Full Name": "full_name"})
	if status != http.StatusAccepted {
		t.Fatalf("run: %d", status)
	}
	if s := importCall(t, e, run.ID, "approve", nil, nil); s != http.StatusAccepted {
		t.Fatalf("approve: %d", s)
	}
	var list struct {
		Data []struct {
			Employer *struct {
				CompanyID string `json:"company_id"`
			} `json:"employer"`
		} `json:"data"`
	}
	if s := e.Call(t, http.MethodGet, "/v1/contacts", nil, nil, &list); s != http.StatusOK {
		t.Fatalf("contacts: %d", s)
	}
	if len(list.Data) != 1 || list.Data[0].Employer != nil {
		t.Fatalf("consumer address acquired employer: %+v", list)
	}
}

func TestCSVReferencesFindEmployersCreatedByEarlierContacts(t *testing.T) {
	for _, ref := range []string{"Orion Research", "orion-research.test"} {
		t.Run(ref, func(t *testing.T) {
			e := setupImportApp(t)
			profile, status := uploadCSV(t, e, "contact", "Name,Email,Company,Company Ref\nDana,dana@orion-research.test,Orion Research,\nRobin,robin@other-work.test,,"+ref+"\n")
			if status != http.StatusOK {
				t.Fatalf("upload: %d", status)
			}
			run, status := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"Name": "full_name", "Email": "email", "Company": "company_name", "Company Ref": "company_ref"})
			if status != http.StatusAccepted {
				t.Fatalf("stage: %d", status)
			}
			predicted := readImportReport(t, e, run.ID)
			if predicted.Disposition.Created != 2 || predicted.Disposition.Skipped != 0 {
				t.Fatalf("preview: %+v", predicted)
			}
			approveAndRead(t, e, run.ID)
			if done := readImportReport(t, e, run.ID); done.Disposition.Created != 2 || done.Disposition.Skipped != 0 {
				t.Fatalf("commit: %+v", done)
			}
			for _, email := range []string{"dana@orion-research.test", "robin@other-work.test"} {
				if employerOf(t, e, email) != "Orion Research" {
					t.Fatalf("employer missing for %s", email)
				}
			}
		})
	}
}

func TestCSVPlannedEmployerDomainsDoNotInventAliasCompanies(t *testing.T) {
	e := setupImportApp(t)
	profile, status := uploadCSV(t, e, "contact", "Name,Email,Company,Company Ref\nDana,dana@orion-alias.test,Orion Research,\nRobin,robin@orion-alias.test,Another Name,\nTaylor,taylor@other.test,,Another Name\n")
	if status != http.StatusOK {
		t.Fatalf("upload: %d", status)
	}
	run, status := createRunWithMapping(t, e, "contact", profile.SourceRef, map[string]string{"Name": "full_name", "Email": "email", "Company": "company_name", "Company Ref": "company_ref"})
	if status != http.StatusAccepted {
		t.Fatalf("stage: %d", status)
	}
	before := readImportReport(t, e, run.ID)
	if before.Disposition.Created != 2 || before.Disposition.Skipped != 1 {
		t.Fatalf("preview: %+v", before)
	}
	approveAndRead(t, e, run.ID)
	after := readImportReport(t, e, run.ID)
	if after.Disposition.Created != 2 || after.Disposition.Skipped != 1 {
		t.Fatalf("commit: %+v", after)
	}
	if employerOf(t, e, "robin@orion-alias.test") != "Orion Research" {
		t.Fatal("domain match was not reused")
	}
}
