// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// A company page in a contact's LinkedIn answers 422 on the wire, under the
// field the request carries.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

const companyPageBody = `https://www.linkedin.com/company/acme`

func TestQuickCaptureWithACompanyPageIsRefusedOnProfileURL(t *testing.T) {
	e := setupDedupe(t)
	h := Handlers{store: e.store}
	rec := httptest.NewRecorder()
	h.QuickCaptureContact(rec, httptest.NewRequestWithContext(e.as(), http.MethodPost, "/v1/contacts/quick-capture",
		strings.NewReader(`{"full_name":"Acme Page","profile_url":"`+companyPageBody+`"}`)), crmcontracts.QuickCaptureContactParams{})
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `"profile_url"`) ||
		!strings.Contains(body, "linkedin_url_not_profile") || strings.Contains(body, `"linkedin_url"`) {
		t.Fatalf("got %d %s, want 422 naming profile_url with linkedin_url_not_profile", rec.Code, body)
	}
}

func TestPatchingAContactToACompanyPageIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	id := e.plainContact(ctx, t, "Sam Example", "sam@example.com")
	h := Handlers{store: e.store}
	rec := httptest.NewRecorder()
	h.UpdateContact(rec, httptest.NewRequestWithContext(ctx, http.MethodPatch, "/v1/contacts/"+id.String(),
		strings.NewReader(`{"social":{"linkedin":"`+companyPageBody+`"}}`)),
		crmcontracts.Id(id), crmcontracts.UpdateContactParams{})
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, "linkedin_url_not_profile") ||
		!strings.Contains(body, `"linkedin_url"`) {
		t.Fatalf("got %d %s, want 422 linkedin_url_not_profile on linkedin_url", rec.Code, body)
	}
	var stored int
	if err := e.store.db.Pool().QueryRow(ctx, `SELECT count(*) FROM contact_social WHERE contact_id = $1`, id).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("a refused patch left %d social rows (err %v)", stored, err)
	}
}
