// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestPartnerMetricRefusalsNameTheMetricBeforeAnyWrite(t *testing.T) {
	for _, tc := range []struct{ metrics, field string }{
		{`{"csat": 90}`, "gate_metrics.csat"},
		{`{"certified_staff": -1}`, "gate_metrics.certified_staff"},
		{`{"certified_staff": 32768}`, "gate_metrics.certified_staff"},
		{`{"certified_staff": 1.5}`, "gate_metrics.certified_staff"},
		{`{"certified_staff": "many"}`, "gate_metrics.certified_staff"},
		{`{"retention_rate": -1}`, "gate_metrics.retention_rate"},
		{`{"retention_rate": 101}`, "gate_metrics.retention_rate"},
		{`{"retention_rate": 99.5}`, "gate_metrics.retention_rate"},
		{`{"retention_rate": true}`, "gate_metrics.retention_rate"},
	} {
		t.Run(tc.metrics, func(t *testing.T) {
			body := `{"partner_kinds":["consulting"],"gate_metrics":` + tc.metrics + `}`
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPut, "/v1/companies/partner", strings.NewReader(body))
			req.Header.Set("If-Match", "1")
			Handlers{}.UpsertPartner(rec, req, crmcontracts.Id(ids.NewV7()), crmcontracts.UpsertPartnerParams{})
			if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), tc.field) {
				t.Fatalf("metric answered %d %s, want 422 naming %s", rec.Code, rec.Body, tc.field)
			}
			var into crmcontracts.UpsertPartnerRequest
			if err := datasource.StrictDecode([]byte(body), &into); err == nil {
				t.Error("provider decoder accepted the same invalid metric")
			}
		})
	}
}

func TestPartnerMetricsAdmitOnlyTheTwoBoundedWholeNumbers(t *testing.T) {
	for _, metrics := range []string{`null`, `{}`, `{"certified_staff":0,"retention_rate":0}`, `{"certified_staff":32767,"retention_rate":100}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/v1/companies/partner", strings.NewReader(`{"gate_metrics":`+metrics+`}`))
		var into crmcontracts.UpsertPartnerRequest
		if !httperr.DecodeClosed(rec, req, &into, nil) {
			t.Errorf("valid metrics %s refused: %d %s", metrics, rec.Code, rec.Body)
		}
	}
}
