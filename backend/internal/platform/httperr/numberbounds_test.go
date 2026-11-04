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

// The home repair preview requires a limit of at least 1, as a plain int. A
// sent 0 is refused; a body that leaves the limit out is the handler's.
func TestDecodeRefusesAHomeRepairPreviewOfZeroRecords(t *testing.T) {
	for body, refused := range map[string]bool{`{"limit":0}`: true, `{"limit":1}`: false, `{}`: false} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/admin/home-repair/preview", strings.NewReader(body))
		var into crmcontracts.PreviewHomeRepairJSONBody
		if Decode(rec, req, &into) == refused {
			t.Errorf("%s: decoded = %v, want refused = %v (%d %s)", body, !refused, refused, rec.Code, rec.Body)
			continue
		}
		if refused && (rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"field":"limit"`)) {
			t.Errorf("%s answered %d %s, want 422 naming limit", body, rec.Code, rec.Body)
		}
	}
}
