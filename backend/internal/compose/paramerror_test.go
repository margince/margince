// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type %q, body %q: want application/problem+json", got, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q is not JSON: %v", rec.Body.String(), err)
	}
	return body.Code
}

func TestAPathTheContractDoesNotDeclareAnswersANotFoundProblem(t *testing.T) {
	router := contractRouter(crmcontracts.Unimplemented{})
	for _, path := range []string{"/v1/nope", "/v1/contacts/"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s answered %d, want 404", path, rec.Code)
		}
		if code := problemCode(t, rec); code != "not_found" {
			t.Errorf("GET %s answered code %q, want not_found", path, code)
		}
	}
}

func TestAMethodThePathDoesNotAnswerNamesTheOnesItDoes(t *testing.T) {
	router := contractRouter(crmcontracts.Unimplemented{})
	cases := map[string]string{
		"/v1/contacts": "GET, POST",
		"/v1/contacts/00000000-0000-0000-0000-000000000001": "GET, PATCH, DELETE",
	}
	for path, allow := range cases {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("PUT %s answered %d, want 405", path, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != allow {
			t.Errorf("PUT %s sent Allow %q, want %q", path, got, allow)
		}
		if code := problemCode(t, rec); code != "method_not_allowed" {
			t.Errorf("PUT %s answered code %q, want method_not_allowed", path, code)
		}
	}
}
