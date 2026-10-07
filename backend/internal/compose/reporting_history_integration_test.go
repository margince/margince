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
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingHTTPCompleteMonthComparisonAndHistoryPagination(t *testing.T) {
	f := reportingBusiness(t)
	handler := reportingHTTPRouter(f)
	october := f.createDeal(t, "October order", 10800000, f.env.stages[20], f.env.Rep1)
	var won ids.UUID
	if err := f.env.owner.QueryRow(f.human, "SELECT id FROM stage WHERE pipeline_id=$1 AND semantic='won'", f.env.pipeline).Scan(&won); err != nil {
		t.Fatal(err)
	}
	f.at = time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	reason := "purchase_order"
	if _, err := f.store.AdvanceDeal(f.writer, ids.From[ids.DealKind](ids.UUID(october.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](won), WonWithoutContractReason: &reason}); err != nil {
		t.Fatal(err)
	}
	selection := f.selection()
	selection.Period = "last_month"
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"bookings_trend"}
	report, err := f.service.CreateReport(f.human, crmcontracts.ReportingReportInput{Name: "Monthly review", Audience: "workspace", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	path := "reports/" + report.Id.String()
	editions := []crmcontracts.ReportingEdition{}
	worker := &reportScheduleSweepWorker{pool: f.env.Pool, now: func() time.Time { return f.at }}
	for _, month := range []time.Month{time.October, time.November} {
		f.at = time.Date(2026, month, 1, 12, 0, 0, 0, time.UTC)
		run, err := f.service.Freeze(f.human, ids.UUID(report.Id), report.Revision, month.String())
		if err != nil {
			t.Fatal(err)
		}
		if err := worker.Work(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		completed, err := f.service.GetExecution(f.human, ids.UUID(run.Id))
		if err != nil || completed.EditionId == nil {
			t.Fatalf("capture failed: %+v %v", completed, err)
		}
		edition, err := f.service.GetEdition(f.human, ids.UUID(*completed.EditionId))
		if err != nil {
			t.Fatal(err)
		}
		editions = append(editions, edition)
	}
	comparisonPath := "editions/compare?left_id=" + editions[0].Id.String() + "&right_id=" + editions[1].Id.String()
	comparison := reportingResponse[crmcontracts.ReportingComparison](t, reportingRequest(f.human, t, handler, http.MethodGet, comparisonPath, nil, -1, http.StatusOK))
	if !comparison.Compatible || len(comparison.Deltas) != 1 || comparison.Deltas[0].Absolute != -10800000 || comparison.Deltas[0].Percentage == nil || *comparison.Deltas[0].Percentage != -50 {
		t.Fatalf("month comparison: compatible=%v, reason=%v, deltas=%+v", comparison.Compatible, comparison.Reason, comparison.Deltas)
	}
	repeated, err := f.service.Compare(f.human, ids.UUID(editions[0].Id), ids.UUID(editions[0].Id))
	if err != nil || repeated.Compatible || len(repeated.Deltas) != 0 {
		t.Fatalf("same-period comparison: %+v %v", repeated, err)
	}
	page := reportingResponse[crmcontracts.ReportingEditionList](t, reportingRequest(f.human, t, handler, http.MethodGet, path+"/editions?limit=1", nil, -1, http.StatusOK))
	if len(page.Data) != 1 || page.NextCursor == nil {
		t.Fatalf("edition cursor: %+v", page)
	}
	next := reportingResponse[crmcontracts.ReportingEditionList](t, reportingRequest(f.human, t, handler, http.MethodGet, path+"/editions?limit=1&cursor="+*page.NextCursor, nil, -1, http.StatusOK))
	if len(next.Data) != 1 || next.Data[0].Id == page.Data[0].Id {
		t.Fatal("edition continuation repeated its boundary")
	}
	reportingAssertToolHistory(t, f, report, editions)
}

func reportingAssertToolHistory(t *testing.T, f *reportingBusinessFixture, report crmcontracts.ReportingReport, editions []crmcontracts.ReportingEdition) {
	t.Helper()
	reads := []agents.ReportingRead{
		{Mode: "editions", ID: ids.UUID(report.Id)},
		{Mode: "compare", ID: ids.UUID(editions[0].Id), RightID: ids.UUID(editions[1].Id)},
		{Mode: "evidence", ID: ids.UUID(editions[0].Id), Reference: crmcontracts.ReportingEvidenceRef{Metric: "bookings_won", ContextId: "interval"}},
	}
	for _, read := range reads {
		answer, err := readReportingResult(f.human, f.service, read)
		if err != nil {
			t.Fatal(err)
		}
		switch read.Mode {
		case "editions":
			if answer.Editions == nil || len(answer.Editions.Data) != 2 {
				t.Fatalf("tool editions: %+v", answer)
			}
		case "compare":
			if answer.Comparison == nil || !answer.Comparison.Compatible {
				t.Fatalf("tool comparison: %+v", answer)
			}
		case "evidence":
			if answer.Evidence == nil || len(answer.Evidence.Rows) != 12 {
				t.Fatalf("tool evidence: %+v", answer)
			}
		}
	}
}

func TestReportingHTTPMutationGrantsCannotBeReplacedByReadAccess(t *testing.T) {
	f := reportingBusiness(t)
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("missing actor")
	}
	grants := map[string]principal.ObjectGrant{}
	for name := range actor.Permissions.Objects {
		grants[name] = principal.ObjectGrant{Read: true}
	}
	actor.Permissions.Objects = grants
	reader := principal.WithActor(f.human, actor)
	handler := reportingHTTPRouter(f)
	id := ids.NewV7().String()
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "reports"},
		{http.MethodPatch, "reports/" + id},
		{http.MethodDelete, "reports/" + id},
		{http.MethodPost, "targets"},
		{http.MethodPatch, "targets/" + id},
		{http.MethodPut, "framework"},
		{http.MethodPost, "reports/" + id + "/schedules"},
		{http.MethodPatch, "schedules/" + id},
		{http.MethodPost, "reports/" + id + "/editions"},
		{http.MethodPost, "executions/" + id + "/retry"},
	} {
		t.Run(request.method+request.path, func(t *testing.T) {
			reportingRequest(reader, t, handler, request.method, request.path, []byte("{}"), 1, http.StatusForbidden)
		})
	}
}
