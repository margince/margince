// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func undeclaredRouter() http.Handler {
	r := chi.NewRouter()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for route := range queryParamTable {
		method, pattern, _ := strings.Cut(route, " ")
		r.With(refuseUndeclaredQueryParameter).Method(method, pattern, ok)
	}
	return r
}

func TestEveryRouteRefusesAQueryKeyItDoesNotDeclare(t *testing.T) {
	h := undeclaredRouter()
	for route := range queryParamTable {
		rec := call(h, route, "zz_not_declared=1")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s ?zz_not_declared answered %d, want 422", route, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), "zz_not_declared") || !strings.Contains(rec.Body.String(), "invalid_parameter") {
			t.Errorf("%s refusal does not name the key with invalid_parameter: %s", route, rec.Body.String())
		}
	}
}

func TestEveryDeclaredQueryParameterIsAdmitted(t *testing.T) {
	h := undeclaredRouter()
	for route, names := range queryParamTable {
		for _, name := range names {
			if rec := call(h, route, url.Values{name: {"1"}}.Encode()); rec.Code != http.StatusOK {
				t.Errorf("%s ?%s answered %d, want 200", route, name, rec.Code)
			}
		}
	}
}

func TestEveryUndeclaredQueryKeyIsNamedInOneRefusal(t *testing.T) {
	rec := call(undeclaredRouter(), "GET /v1/deals", "limit=5&stauts=open&zzz=1")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("answered %d, want 422", rec.Code)
	}
	body := rec.Body.String()
	for _, key := range []string{"stauts", "zzz"} {
		if !strings.Contains(body, key) {
			t.Errorf("the refusal does not name %s: %s", key, body)
		}
	}
	if strings.Contains(body, `"limit"`) {
		t.Errorf("the refusal names the declared key limit: %s", body)
	}
}

func TestCustomFieldFiltersAndRoutesOutsideTheContractAreLeftAlone(t *testing.T) {
	if rec := call(undeclaredRouter(), "GET /v1/deals", "cf_score=3"); rec.Code != http.StatusOK {
		t.Errorf("a cf_ filter answered %d, want 200", rec.Code)
	}
	handler := refuseUndeclaredQueryParameter(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?client_id=x", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("a route with no contract entry answered %d, want 200", rec.Code)
	}
}

// The expected set comes from the plain-YAML reading of the contract, so a
// table the generator wrote short cannot vouch for itself.
func TestTheTableHoldsEveryQueryParameterTheContractDeclares(t *testing.T) {
	leftAlone := func(operation map[string]any) bool {
		security, hasSecurity := operation["security"].([]any)
		open, _ := operation["x-open-query"].(bool)
		return open || (hasSecurity && len(security) == 0)
	}
	want := contractQueryParameters(t, leftAlone, func(func(any) map[string]any, map[string]any) bool { return true })
	got := map[string]bool{}
	for route, names := range queryParamTable {
		for _, name := range names {
			got[route+" ?"+name] = true
		}
	}
	if len(want) < 400 {
		t.Fatalf("the contract reading found only %d checked query parameters; the reader is short", len(want))
	}
	for key := range want {
		if !got[key] {
			t.Errorf("the contract declares %s and the generated table lacks it", key)
		}
	}
	for key := range got {
		if !want[key] {
			t.Errorf("the generated table holds %s and the contract declares no such parameter", key)
		}
	}
}

func TestAMalformedQueryStringIsRefusedNotReadAsNoFilter(t *testing.T) {
	h := undeclaredRouter()
	for _, query := range []string{"st%ZZatus=open", "status=%ZZ", "limit=5&%=1", "a=1;b=2"} {
		rec := call(h, "GET /v1/deals", query)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "invalid_parameter") {
			t.Errorf("?%s answered %d %s, want 422 invalid_parameter", query, rec.Code, rec.Body.String())
		}
	}
}
