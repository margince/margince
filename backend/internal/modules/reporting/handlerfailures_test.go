// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestReportingMutationHandlersRejectMalformedBodiesBeforeCallingTheService(t *testing.T) {
	handlers := NewHandlers(nil)
	id := crmcontracts.Id{}
	cases := []struct {
		name string
		call http.HandlerFunc
	}{
		{"create report", handlers.CreateReportingReport},
		{"create target", handlers.CreateReportingTarget},
		{"update report", func(w http.ResponseWriter, r *http.Request) {
			handlers.UpdateReportingReport(w, r, id, crmcontracts.UpdateReportingReportParams{})
		}},
		{"update target", func(w http.ResponseWriter, r *http.Request) {
			handlers.UpdateReportingTarget(w, r, id, crmcontracts.UpdateReportingTargetParams{})
		}},
		{"publish framework", func(w http.ResponseWriter, r *http.Request) {
			handlers.PublishReportingFramework(w, r, crmcontracts.PublishReportingFrameworkParams{})
		}},
		{"create schedule", func(w http.ResponseWriter, r *http.Request) { handlers.CreateReportingSchedule(w, r, id) }},
		{"update schedule", func(w http.ResponseWriter, r *http.Request) {
			handlers.UpdateReportingSchedule(w, r, id, crmcontracts.UpdateReportingScheduleParams{})
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/analytics", strings.NewReader("{"))
			response := httptest.NewRecorder()
			test.call(response, request)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("malformed body: %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestReportingRevisionHandlersRequireAnObservedVersion(t *testing.T) {
	handlers := NewHandlers(nil)
	id := crmcontracts.Id{}
	calls := []http.HandlerFunc{
		func(w http.ResponseWriter, r *http.Request) {
			handlers.UpdateReportingReport(w, r, id, crmcontracts.UpdateReportingReportParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handlers.UpdateReportingTarget(w, r, id, crmcontracts.UpdateReportingTargetParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handlers.PublishReportingFramework(w, r, crmcontracts.PublishReportingFrameworkParams{})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handlers.UpdateReportingSchedule(w, r, id, crmcontracts.UpdateReportingScheduleParams{})
		},
	}
	for _, version := range []string{"", "invalid"} {
		for _, call := range calls {
			request := httptest.NewRequest(http.MethodPatch, "/analytics", strings.NewReader("{}"))
			if version != "" {
				request.Header.Set("If-Match", version)
			}
			response := httptest.NewRecorder()
			call(response, request)
			want := http.StatusUnprocessableEntity
			if version == "" {
				want = http.StatusBadRequest
			}
			if response.Code != want {
				t.Fatalf("missing/invalid version: %d %s", response.Code, response.Body.String())
			}
		}
	}
}

func TestReportingListHandlersRejectMalformedContinuationTokens(t *testing.T) {
	handlers := NewHandlers(nil)
	cursor := "not-an-id"
	id := crmcontracts.Id{}
	calls := []http.HandlerFunc{
		func(w http.ResponseWriter, r *http.Request) {
			handlers.ListReportingReports(w, r, crmcontracts.ListReportingReportsParams{Cursor: &cursor})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handlers.ListReportingTargets(w, r, crmcontracts.ListReportingTargetsParams{Cursor: &cursor})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handlers.ListReportingEditions(w, r, id, crmcontracts.ListReportingEditionsParams{Cursor: &cursor})
		},
		func(w http.ResponseWriter, r *http.Request) {
			handlers.ListReportingExecutions(w, r, id, crmcontracts.ListReportingExecutionsParams{Cursor: &cursor})
		},
	}
	for _, call := range calls {
		response := httptest.NewRecorder()
		call(response, httptest.NewRequest(http.MethodGet, "/analytics", nil))
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid cursor: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestReportingQueryRejectsMalformedScopeAndDatesBeforeEvaluation(t *testing.T) {
	handlers := NewHandlers(nil)
	for _, query := range []string{"scope_id=invalid", "pipeline_id=invalid", "start_at=invalid", "start_at=2026-09-01T00:00:00Z&end_at=invalid"} {
		for _, evidence := range []bool{false, true} {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/analytics/evaluate?"+query, nil)
			if evidence {
				handlers.GetReportingEvidence(response, request, crmcontracts.GetReportingEvidenceParams{})
			} else {
				handlers.EvaluateReporting(response, request, crmcontracts.EvaluateReportingParams{})
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("malformed selection: %d %s", response.Code, response.Body.String())
			}
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/analytics/evaluate?period=custom&target_basis=fiscal_quarter&close_window=all_open&start_at=2026-09-01T00:00:00Z&end_at=2026-09-08T00:00:00Z", nil)
	selection, err := SelectionFromQuery(request.URL.Query())
	if err != nil || selection.Interval == nil || selection.Interval.EndAt.Sub(selection.Interval.StartAt).Hours() != 168 || selection.TargetBasis != "fiscal_quarter" || selection.CloseWindow != "all_open" {
		t.Fatalf("custom selection: %+v %v", selection, err)
	}
}
