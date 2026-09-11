// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// The transport for the company-360 evidence reads: the handlers wrap the
// gated store reads onto the wire shape, answer [] (never null) for a company
// with no evidence, and existence-hide a foreign/absent company as 404.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestListCompanyFactsHandler(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := seedCompanyWithEvidence(ctx, t, e)
	h := Handlers{store: e.store}

	// Populated company → 200 with its facts.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/companies/x/facts", nil).WithContext(ctx)
	h.ListCompanyFacts(rec, req, crmcontracts.Id(companyID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("facts handler status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var facts crmcontracts.CompanyFactListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &facts); err != nil {
		t.Fatalf("decode facts response: %v", err)
	}
	if len(facts.Data) != 2 {
		t.Fatalf("facts handler returned %d facts, want 2", len(facts.Data))
	}

	// Company with no facts → 200 with an empty array, never null.
	empty, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Bare GmbH", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ListCompanyFacts(rec, req, empty.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty facts status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != `{"data":[]}`+"\n" && body != `{"data":[]}` {
		t.Fatalf("empty facts body = %q, want an empty data array (never null)", body)
	}

	// A foreign/absent company is existence-hidden as 404.
	rec = httptest.NewRecorder()
	h.ListCompanyFacts(rec, req, crmcontracts.Id(ids.NewV7()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("absent-company facts status = %d, want 404", rec.Code)
	}
}

func TestListCompanyProfileFieldsHandler(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := seedCompanyWithEvidence(ctx, t, e)
	h := Handlers{store: e.store}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/companies/x/profile-fields", nil).WithContext(ctx)
	h.ListCompanyProfileFields(rec, req, crmcontracts.Id(companyID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("profile-fields handler status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var fields crmcontracts.CompanyProfileFieldListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode profile-fields response: %v", err)
	}
	if len(fields.Data) != 2 {
		t.Fatalf("profile-fields handler returned %d fields, want 2", len(fields.Data))
	}

	// Company with no profile fields → 200 with an empty array, never null.
	empty, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Bare GmbH", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ListCompanyProfileFields(rec, req, empty.Id)
	if rec.Code != http.StatusOK {
		t.Fatalf("empty profile-fields status = %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); body != `{"data":[]}`+"\n" && body != `{"data":[]}` {
		t.Fatalf("empty profile-fields body = %q, want an empty data array (never null)", body)
	}

	rec = httptest.NewRecorder()
	h.ListCompanyProfileFields(rec, req, crmcontracts.Id(ids.NewV7()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("absent-company profile-fields status = %d, want 404", rec.Code)
	}
}
