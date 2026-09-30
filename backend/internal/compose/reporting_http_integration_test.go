// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
)

func reportingHTTPRouter(f *reportingBusinessFixture) http.Handler {
	authHandlers := identity.NewHandlers(identity.NewService(f.env.Pool)).WithReportingAvailable(true)
	server := newServer(f.env.Pool, slog.New(slog.DiscardHandler), authHandlers, deals.NewHandlers(InstallationDB(f.env.Pool), DealsInstallation()))
	server.reportingHandlers = reporting.NewHandlers(f.service)
	server.reportingExportHandlers = reportingExportHandlers{service: f.service}
	return crmcontracts.HandlerFromMuxWithBaseURL(server, chi.NewRouter(), "/v1")
}

func reportingRequest(ctx context.Context, t *testing.T, handler http.Handler, method, path string, body []byte, version int64, status int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "/v1/analytics/"+path, bytes.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", path)
	if version >= 0 {
		request.Header.Set("If-Match", fmt.Sprint(version))
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, response.Code, status, response.Body.String())
	}
	return response
}

func reportingPayload[T any](t *testing.T, value T) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func reportingResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestReportingHTTPRevisionsSchedulesAndFrozenHistory(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	input := crmcontracts.ReportingReportInput{Name: "Operating review", Audience: "workspace", Selection: f.selection()}
	created := reportingRequest(f.human, t, handler, http.MethodPost, "reports", reportingPayload(t, input), -1, http.StatusCreated)
	report := reportingResponse[crmcontracts.ReportingReport](t, created)
	path := "reports/" + report.Id.String()
	input.Name = "Revised operating review"
	revised := reportingRequest(f.human, t, handler, http.MethodPatch, path, reportingPayload(t, input), report.Version, http.StatusOK)
	report = reportingResponse[crmcontracts.ReportingReport](t, revised)
	if report.Revision != 2 || report.Name != input.Name {
		t.Fatalf("report revision: %+v", report)
	}
	reportingRequest(f.human, t, handler, http.MethodPatch, path, reportingPayload(t, input), 1, http.StatusConflict)
	fetched := reportingResponse[crmcontracts.ReportingReport](t, reportingRequest(f.human, t, handler, http.MethodGet, path, nil, -1, http.StatusOK))
	if fetched.Version != report.Version {
		t.Fatal("stale update replaced the report")
	}
	live := reportingResponse[crmcontracts.ReportingEvaluation](t, reportingRequest(f.human, t, handler, http.MethodGet, path+"/evaluation?revision=1", nil, -1, http.StatusOK))
	if live.Metrics[0].Value == nil || *live.Metrics[0].Value != 21600000 {
		t.Fatalf("saved selection: %+v", live.Metrics)
	}
	if _, err := f.env.owner.Exec(f.human, `INSERT INTO retention_policy(object_type,retain_days,action,enabled) VALUES('report_edition',730,'erase',true)`); err != nil {
		t.Fatal(err)
	}
	scheduleInput := crmcontracts.ReportingScheduleInput{ReportRevision: report.Revision, Frequency: "monthly", Day: 31, LocalTime: "09:00", Enabled: true}
	schedule := reportingResponse[crmcontracts.ReportingSchedule](t, reportingRequest(f.human, t, handler, http.MethodPost, path+"/schedules", reportingPayload(t, scheduleInput), -1, http.StatusCreated))
	scheduleInput.LocalTime = "10:30"
	updated := reportingResponse[crmcontracts.ReportingSchedule](t, reportingRequest(f.human, t, handler, http.MethodPatch, "schedules/"+schedule.Id.String(), reportingPayload(t, scheduleInput), schedule.Version, http.StatusOK))
	if updated.Version != schedule.Version+1 || updated.Definition.LocalTime != "10:30" {
		t.Fatalf("schedule revision: %+v", updated)
	}
	reportingRequest(f.human, t, handler, http.MethodPatch, "schedules/"+schedule.Id.String(), reportingPayload(t, scheduleInput), schedule.Version, http.StatusConflict)
	execution := reportingResponse[crmcontracts.ReportingExecution](t, reportingRequest(f.human, t, handler, http.MethodPost, path+"/editions", nil, -1, http.StatusAccepted))
	worker := &reportScheduleSweepWorker{enabled: true, pool: f.env.Pool, now: func() time.Time { return f.at }}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed := reportingResponse[crmcontracts.ReportingExecution](t, reportingRequest(f.human, t, handler, http.MethodGet, "executions/"+execution.Id.String(), nil, -1, http.StatusOK))
	if completed.EditionId == nil || completed.Status != "succeeded" {
		t.Fatalf("publication: %+v", completed)
	}
	editionPath := "editions/" + completed.EditionId.String()
	edition := reportingResponse[crmcontracts.ReportingEdition](t, reportingRequest(f.human, t, handler, http.MethodGet, editionPath, nil, -1, http.StatusOK))
	if edition.ReportRevision != report.Revision {
		t.Fatal("edition used the wrong revision")
	}
	reportingAssertLibrary(t, f, handler, report.Id.String())
	evidence := reportingResponse[crmcontracts.ReportingEvidence](t, reportingRequest(f.human, t, handler, http.MethodGet, editionPath+"/evidence?metric=bookings_won&context_id=interval", nil, -1, http.StatusOK))
	if len(evidence.Rows) != 12 {
		t.Fatalf("frozen contribution count: %d", len(evidence.Rows))
	}
	export := reportingRequest(f.human, t, handler, http.MethodGet, editionPath+"/export.csv", nil, -1, http.StatusOK)
	if !strings.Contains(export.Body.String(), "21600000") {
		t.Fatal("export lost frozen bookings")
	}
	reportingRequest(f.human, t, handler, http.MethodDelete, path, nil, -1, http.StatusOK)
	schedules := reportingResponse[crmcontracts.ReportingScheduleList](t, reportingRequest(f.human, t, handler, http.MethodGet, path+"/schedules", nil, -1, http.StatusOK))
	if len(schedules.Data) != 1 || schedules.Data[0].Definition.Enabled {
		t.Fatal("archive did not pause schedules")
	}
	reportingRequest(f.human, t, handler, http.MethodGet, editionPath, nil, -1, http.StatusOK)
	library := reportingResponse[crmcontracts.ReportingReportList](t, reportingRequest(f.human, t, handler, http.MethodGet, "reports", nil, -1, http.StatusOK))
	if len(library.Data) != 0 {
		t.Fatal("archived report remained in library")
	}
}

func reportingAssertLibrary(t *testing.T, f *reportingBusinessFixture, handler http.Handler, id string) {
	t.Helper()
	library := reportingResponse[crmcontracts.ReportingReportList](t, reportingRequest(f.human, t, handler, http.MethodGet, "reports?scheduled=true", nil, -1, http.StatusOK))
	if len(library.Data) != 1 {
		t.Fatalf("scheduled library: %+v", library)
	}
	summary := library.Data[0]
	if summary.Id.String() != id || summary.EditionCount == nil || *summary.EditionCount != 1 || summary.LatestCapturedAt == nil || summary.NextDueAt == nil || summary.Cadence == nil || *summary.Cadence != "monthly" {
		t.Fatalf("library summary: %+v", summary)
	}
	editions := reportingResponse[crmcontracts.ReportingEditionList](t, reportingRequest(f.human, t, handler, http.MethodGet, "reports/"+id+"/editions?limit=1", nil, -1, http.StatusOK))
	runs := reportingResponse[crmcontracts.ReportingExecutionList](t, reportingRequest(f.human, t, handler, http.MethodGet, "reports/"+id+"/executions?limit=1", nil, -1, http.StatusOK))
	if len(editions.Data) != 1 || len(runs.Data) != 1 || runs.Data[0].Status != "succeeded" {
		t.Fatal("publication history is incomplete")
	}
}

func TestReportingHTTPFrameworkAndTargetVersions(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	framework := reportingResponse[crmcontracts.ReportingFramework](t, reportingRequest(f.human, t, handler, http.MethodGet, "framework", nil, -1, http.StatusOK))
	framework.Definition.Template, framework.Definition.Reason = "sdr", "Use agreed SDR outcomes"
	revised := reportingResponse[crmcontracts.ReportingFramework](t, reportingRequest(f.human, t, handler, http.MethodPut, "framework", reportingPayload(t, framework.Definition), framework.Version, http.StatusOK))
	if revised.Revision != framework.Revision+1 {
		t.Fatal("framework revision did not advance")
	}
	reportingRequest(f.human, t, handler, http.MethodPut, "framework", reportingPayload(t, framework.Definition), framework.Version, http.StatusConflict)
	input := f.target.Definition
	input.Metric, input.Value, input.PipelineId = "meetings_held", 40, nil
	target := reportingResponse[crmcontracts.ReportingTarget](t, reportingRequest(f.human, t, handler, http.MethodPost, "targets", reportingPayload(t, input), -1, http.StatusCreated))
	input.Value, input.Reason = 45, "Additional capacity"
	path := "targets/" + target.Id.String()
	corrected := reportingResponse[crmcontracts.ReportingTarget](t, reportingRequest(f.human, t, handler, http.MethodPatch, path, reportingPayload(t, input), target.Version, http.StatusOK))
	if corrected.Definition.Value != 45 || corrected.Revision != 2 || corrected.Unit != "count" {
		t.Fatalf("corrected target: %+v", corrected)
	}
	reportingRequest(f.human, t, handler, http.MethodPatch, path, reportingPayload(t, input), target.Version, http.StatusConflict)
	got := reportingResponse[crmcontracts.ReportingTarget](t, reportingRequest(f.human, t, handler, http.MethodGet, path, nil, -1, http.StatusOK))
	if got.Version != corrected.Version {
		t.Fatal("stale target write was accepted")
	}
	list := reportingResponse[crmcontracts.ReportingTargetList](t, reportingRequest(f.human, t, handler, http.MethodGet, "targets?limit=1", nil, -1, http.StatusOK))
	if len(list.Data) != 1 || list.NextCursor == nil {
		t.Fatalf("target pagination: %+v", list)
	}
	next := reportingResponse[crmcontracts.ReportingTargetList](t, reportingRequest(f.human, t, handler, http.MethodGet, "targets?limit=1&cursor="+*list.NextCursor, nil, -1, http.StatusOK))
	if len(next.Data) != 1 || next.Data[0].Id == list.Data[0].Id {
		t.Fatal("target cursor repeated a row")
	}
}
