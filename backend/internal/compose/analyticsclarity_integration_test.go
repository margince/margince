// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCurrentUnansweredBacklogDoesNotBecomeAnArrivalCohort(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.PipelineId = nil
	selection.Metrics = []crmcontracts.ReportingMetricID{reportingLeadsAwaitingResponse, reportingLeadsAwaitingResponseNow}
	selection.Blocks = nil
	for _, period := range []crmcontracts.ReportingSelectionPeriod{"this_week", "last_week"} {
		selection.Period = period
		result, err := f.service.Evaluate(f.human, selection)
		if err != nil {
			t.Fatal(err)
		}
		for _, metric := range result.Metrics {
			want := float64(0)
			if metric.Id == reportingLeadsAwaitingResponseNow {
				want = 1
			}
			if metric.Value == nil || *metric.Value != want {
				t.Fatalf("%s %s: %+v, want %v", period, metric.Id, metric, want)
			}
			ref := metric.Evidence
			rows, err := f.service.Evidence(f.human, selection, ref.Metric, ref.ContextId, "", nil, nil, 10, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
			if err != nil || len(rows.Rows) != int(want) {
				t.Fatalf("%s evidence: %+v %v", metric.Id, rows, err)
			}
		}
	}
}

func TestMissingAmountActionOpensOnlyTheQuarterSubset(t *testing.T) {
	f := reportingBusiness(t)
	closeAt := f.at.AddDate(0, 0, 1)
	owner := ids.From[ids.UserKind](f.env.Rep1)
	first, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{Name: "Quarter prospect without an amount", PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), StageID: ids.From[ids.StageKind](f.env.stages[20]), OwnerID: &owner, ExpectedClose: &closeAt, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	selection := f.selection()
	selection.Period = "this_quarter"
	selection.Metrics = []crmcontracts.ReportingMetricID{reportingForecastLanding, reportingOpenPipeline}
	selection.Blocks = nil
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range result.Metrics {
		if metric.Id != reportingForecastLanding {
			continue
		}
		basis := metric.ForecastBasis
		if basis == nil || basis.OpenCount == nil || *basis.OpenCount != int64(len(f.open)+1) || basis.OpenEvidence == nil {
			t.Fatalf("quarter open population: %+v", basis)
		}
		ref := basis.OpenEvidence
		var cursor *string
		count := 0
		for {
			page, err := f.service.Evidence(f.human, selection, ref.Metric, ref.ContextId, "", nil, cursor, 5, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
			if err != nil {
				t.Fatal(err)
			}
			count += len(page.Rows)
			cursor = page.NextCursor
			if cursor == nil {
				break
			}
		}
		if int64(count) != *basis.OpenCount {
			t.Fatalf("quarter records %d, headline %d", count, *basis.OpenCount)
		}
	}
	if result.OverviewActions == nil {
		t.Fatal("missing remedy action")
	}
	for _, action := range *result.OverviewActions {
		if action.Id != "forecast_missing_amount" {
			continue
		}
		if action.Count != 1 {
			t.Fatalf("action counts %d, want one unpriced deal", action.Count)
		}
		ref := action.Evidence
		evidence, err := f.service.Evidence(f.human, selection, ref.Metric, ref.ContextId, "", nil, nil, 1, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
		if err != nil || len(evidence.Rows) != 1 || evidence.Rows[0].SourceId == nil || *evidence.Rows[0].SourceId != first.Id || evidence.NextCursor != nil || evidence.Rows[0].OwnerLabel == nil || evidence.Rows[0].StageLabel == nil || evidence.Rows[0].OccurredAt == nil {
			t.Fatalf("subset: %+v %v", evidence, err)
		}
		return
	}
	t.Fatal("missing amount remedy was not advertised")
}

func TestOpenPipelineKeepsDealsWhoseExpectedCloseIsOutsideTheReportPeriod(t *testing.T) {
	f := reportingBusiness(t)
	selection := f.selection()
	selection.CloseWindow = "all_open"
	selection.Metrics = []crmcontracts.ReportingMetricID{reportingOpenPipeline}
	selection.Blocks = nil
	for _, period := range []crmcontracts.ReportingSelectionPeriod{"this_week", "last_month"} {
		selection.Period = period
		result, err := f.service.Evaluate(f.human, selection)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Metrics) != 1 || result.Metrics[0].Value == nil || *result.Metrics[0].Value != 88000000 {
			t.Fatalf("%s current stock: %+v", period, result.Metrics)
		}
	}
}

func TestAssessmentUpcomingWorkUsesCurrentClockAndHonorsIndividualPrivacy(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSales)
	f.logMeeting(t, "Future prospect meeting", f.at.Add(24*time.Hour), "booked")
	service := newSalesAssessments(nil, newSalesDecisions(InstallationDB(f.e.Pool)), InstallationDB(f.e.Pool))
	service.now = func() time.Time { return f.at }
	evaluation := crmcontracts.ReportingEvaluation{Context: crmcontracts.ReportingContext{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.e.Rep1)}, Timezone: "Europe/Berlin", EvaluatedAt: f.at.AddDate(0, -1, 0)}}
	f.e.showIndividualPerformance(t, false)
	cells, err := service.upcomingFacts(f.ctx, evaluation)
	if err != nil || len(cells) != 1 || cells[0].Value != "1 count" {
		t.Fatalf("own current work: %+v %v", cells, err)
	}
	evaluation.Context.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.e.Rep3)}
	cells, err = service.upcomingFacts(f.ctx, evaluation)
	if err == nil && len(cells) > 0 {
		t.Fatalf("private colleague work: %+v", cells)
	}
	kind := crmcontracts.ListSdrUpcomingMeetingsParamsScopeKindWorkspace
	recorder := httptest.NewRecorder()
	handler := newSdrSummaryHandlers(InstallationDB(f.e.Pool), func() time.Time { return f.at })
	handler.ListSdrUpcomingMeetings(recorder, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(f.ctx), crmcontracts.ListSdrUpcomingMeetingsParams{ScopeKind: &kind})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("private team list: %d", recorder.Code)
	}
	evaluation.Context.Scope = crmcontracts.ReportingScope{Kind: "workspace"}
	cells, err = service.upcomingFacts(f.ctx, evaluation)
	if err != nil || len(cells) != 0 {
		t.Fatalf("private team work: %+v %v", cells, err)
	}
}

func TestForecastEvidenceDatesKeepPrivateTotalsAndRemediesConsistent(t *testing.T) {
	f := reportingBusiness(t)
	asOf := f.at
	f.at = asOf.AddDate(0, 0, -28)
	colleagues := append(f.env.addMembers(t, f.env.Team1, "first@example.test", "second@example.test"), f.env.Rep3)
	for i, colleague := range colleagues {
		closeAt := asOf.AddDate(0, 0, -7*i)
		owner := ids.From[ids.UserKind](colleague)
		_, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{Name: "Quarter deal without an amount", PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), StageID: ids.From[ids.StageKind](f.env.stages[20]), OwnerID: &owner, ExpectedClose: &closeAt, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
	}
	f.at = asOf
	f.env.showIndividualPerformance(t, false)
	selection := f.selection()
	selection.Period = "this_quarter"
	selection.Metrics = []crmcontracts.ReportingMetricID{reportingOpenPipeline, reportingStageAge, reportingForecastLanding}
	selection.Blocks = nil
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range result.Metrics {
		if metric.Coverage.Withheld {
			t.Fatalf("display dates withheld %s: %+v", metric.Id, metric.Coverage)
		}
	}
	if result.OverviewActions == nil {
		t.Fatal("missing remedy")
	}
	for _, action := range *result.OverviewActions {
		if action.Id != "forecast_missing_amount" {
			continue
		}
		ref := action.Evidence
		page, err := f.service.Evidence(f.human, selection, ref.Metric, ref.ContextId, "", nil, nil, 10, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
		if err != nil || len(page.Rows) != action.Count || action.Count != 3 {
			t.Fatalf("remedy count and records: %+v %+v %v", action, page, err)
		}
		for _, row := range page.Rows {
			if row.OwnerLabel != nil || row.OwnerId != nil || row.OccurredAt == nil {
				t.Fatalf("private dated evidence: %+v", row)
			}
		}
		return
	}
	t.Fatal("missing unpriced-quarter remedy")
}
