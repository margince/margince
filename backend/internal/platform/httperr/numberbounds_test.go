// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The contract bounds the employment backfill batch to 1..25. The shared
// decode refuses a size outside it with the code a list route's limit answers.
func TestDecodeRefusesAnEmploymentBackfillLimitOutsideItsRange(t *testing.T) {
	for _, limit := range []string{"1", "10", "25"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/employment-import/backfill", strings.NewReader(`{"limit":`+limit+`}`))
		var into crmcontracts.BackfillEmploymentImportJSONRequestBody
		if !Decode(rec, req, &into) {
			t.Errorf("limit %s refused: %d %s", limit, rec.Code, rec.Body)
		}
	}
	for _, limit := range []string{"0", "-1", "26", "1000"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/employment-import/backfill", strings.NewReader(`{"limit":`+limit+`}`))
		var into crmcontracts.BackfillEmploymentImportJSONRequestBody
		if Decode(rec, req, &into) {
			t.Errorf("limit %s accepted", limit)
			continue
		}
		body := rec.Body.String()
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(body, `"field":"limit"`) || !strings.Contains(body, `"code":"out_of_range"`) {
			t.Errorf("limit %s answered %d %s, want 422 naming limit as out_of_range", limit, rec.Code, body)
		}
	}
}
