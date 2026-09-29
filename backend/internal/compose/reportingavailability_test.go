// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReportingAvailabilityKeepsLegacyAnalyticsAndPauseReachable(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, path := range []string{"/v1/analytics/metrics", "/v1/analytics/evaluate.csv", "/v1/analytics/editions/example/evidence", "/v1/analytics/reports"} {
			rec := httptest.NewRecorder()
			reached := false
			reportingAvailability(enabled)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { reached = true; w.WriteHeader(http.StatusNoContent) })).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if reached != enabled || (!enabled && rec.Code != http.StatusNotFound) {
				t.Fatalf("enabled=%v path=%s: reached=%v status=%d", enabled, path, reached, rec.Code)
			}
		}
	}
	for _, path := range []string{"/v1/analytics/context", "/v1/analytics/reports/render", "/v1/admin/reporting/pause", "/v1/reports/forecast"} {
		if reportingRoute(path) {
			t.Fatalf("rollback hid independent route %s", path)
		}
	}
}

func TestReportingDisabledOffersNoMCPToolAndDoesNoWorkerIO(t *testing.T) {
	registry := registryWithGate(InstallationDB(nil), nil, nil, SendPath{}, companyEnricher{}, nil, nil, nil, nil, slog.Default(), registryFeatures{})
	if _, found := registry.Spec("read_reporting"); found {
		t.Fatal("disabled reporting was offered to MCP")
	}
	worker := &reportScheduleSweepWorker{enabled: false}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}
