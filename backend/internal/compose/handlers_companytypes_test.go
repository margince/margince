// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestTheCompanyTypeDirectoryListsEveryDeclaredType(t *testing.T) {
	t.Parallel()
	composed := &Extensions{units: []extension.Extension{
		{Name: "beta", CompanyTypes: []extension.CompanyType{{Type: "reseller", Label: "Beta reseller"}}},
		{Name: "alpha", CompanyTypes: []extension.CompanyType{{Type: "holding", Label: "Holding"}}},
	}}

	rec := httptest.NewRecorder()
	channelProvidersHandlers{extensions: composed}.ListCompanyTypes(rec, httptest.NewRequest(http.MethodGet, "/v1/company-types", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got crmcontracts.CompanyTypeDirectory
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []crmcontracts.CompanyTypeOption{
		{Value: "ext:alpha:holding", Label: "Holding", Unit: "alpha"},
		{Value: "ext:beta:reseller", Label: "Beta reseller", Unit: "beta"},
	}
	if len(got.Data) != len(want) || got.Data[0] != want[0] || got.Data[1] != want[1] {
		t.Errorf("data = %+v, want %+v", got.Data, want)
	}
}

func TestTheCompanyTypeDirectoryIsAnEmptyListWhenNothingIsDeclared(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	channelProvidersHandlers{}.ListCompanyTypes(rec, httptest.NewRequest(http.MethodGet, "/v1/company-types", nil))
	if body := rec.Body.String(); body != "{\"data\":[]}\n" && body != "{\"data\":[]}" {
		t.Errorf("body = %q, want an empty data array and not null", body)
	}
}
