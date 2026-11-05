// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The contract caps the asked address at 320 characters, and the transport refuses
// a longer one before any store is reached, naming the field.
func TestAnOverlongAddressIsRefusedNamingEmail(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/companies/for-address", nil)
	Handlers{}.GetCompanyForAddress(rec, req, crmcontracts.GetCompanyForAddressParams{
		Email: "dana@" + strings.Repeat("ä", maxAddressQuery),
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	problem := problemOf(t, rec)
	if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != "email" ||
		problem.Details.Errors[0].Code != "too_long" {
		t.Fatalf("refusal = %+v, want email too_long", problem)
	}
}
