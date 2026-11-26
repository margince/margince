// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/corefeatures"
)

func TestDisabledReportSelectionsHideKeptEditionsUntilReenabled(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	input := crmcontracts.ReportingReportInput{Name: "Kept report", Audience: "workspace", Selection: f.selection()}
	report := reportingResponse[crmcontracts.ReportingReport](t, reportingRequest(f.human, t, handler, http.MethodPost, "reports", reportingPayload(t, input), -1, http.StatusCreated))
	if _, err := f.env.owner.Exec(f.human, `INSERT INTO retention_policy(object_type,retain_days,action,enabled) VALUES('report_edition',730,'erase',true)`); err != nil {
		t.Fatal(err)
	}
	path := "reports/" + report.Id.String()
	execution := reportingResponse[crmcontracts.ReportingExecution](t, reportingRequest(f.human, t, handler, http.MethodPost, path+"/editions", nil, -1, http.StatusAccepted))
	worker := &reportScheduleSweepWorker{pool: f.env.Pool, now: func() time.Time { return f.at }}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed := reportingResponse[crmcontracts.ReportingExecution](t, reportingRequest(f.human, t, handler, http.MethodGet, "executions/"+execution.Id.String(), nil, -1, http.StatusOK))
	if completed.EditionId == nil {
		t.Fatalf("no kept edition: %+v", completed)
	}
	editionPath := "editions/" + completed.EditionId.String()
	disabled := corefeatures.New([]string{"deals"}).Context(f.human)
	reportingRequest(disabled, t, handler, http.MethodGet, path, nil, -1, http.StatusNotFound)
	reportingRequest(disabled, t, handler, http.MethodGet, editionPath, nil, -1, http.StatusNotFound)
	library, err := f.service.ListReports(disabled, nil, 100, false)
	if err != nil || len(library.Data) != 0 {
		t.Fatalf("disabled report library: %+v, %v", library, err)
	}
	reportingRequest(f.human, t, handler, http.MethodGet, path, nil, -1, http.StatusOK)
	kept := reportingResponse[crmcontracts.ReportingEdition](t, reportingRequest(f.human, t, handler, http.MethodGet, editionPath, nil, -1, http.StatusOK))
	if len(kept.Evaluation.Metrics) == 0 {
		t.Fatal("reenabling lost the kept figures")
	}
}

func TestAnEnabledActivityReportOmitsTheDisabledDealPopulation(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.PipelineId = nil
	selection.Metrics = []crmcontracts.ReportingMetricID{reportingMeetingsHeld}
	selection.Blocks = []crmcontracts.ReportingBlockKind{}
	before, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if before.Context.ScopeHoldsDeals == nil {
		t.Fatal("enabled deal population was not reported")
	}
	disabled := corefeatures.New([]string{"deals"}).Context(f.human)
	after, err := f.service.Evaluate(disabled, selection)
	if err != nil {
		t.Fatal(err)
	}
	if after.Context.ScopeHoldsDeals != nil || len(after.Metrics) != 1 {
		t.Fatalf("activity report disclosed the disabled deal population: %+v", after)
	}
}
