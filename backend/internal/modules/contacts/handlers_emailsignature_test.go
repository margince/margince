// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A save with no body is refused before the store is reached. Empty text clears
// a signature, so a forgotten field must not read as that request.
func TestSavingASignatureWithNoBodyIsRefusedNamingTheField(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"an empty object": `{}`,
		"a null body":     `{"body":null}`,
		"only a title":    `{"title":"Head of sales"}`,
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/me/email-signature", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		Handlers{}.SaveMyEmailSignature(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d, want 422", name, rec.Code)
			continue
		}
		problem := problemOf(t, rec)
		if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != "body" {
			t.Errorf("%s: refusal names %+v, want the body field", name, problem.Details.Errors)
		}
	}
}
