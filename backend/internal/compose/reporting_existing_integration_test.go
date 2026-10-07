// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportingExistingWinsMatchTheDealOwnerAndTarget(t *testing.T) {
	f := reportingBusiness(t)
	// Older installations have the deals, but neither attribution column existed.
	if _, err := f.env.owner.Exec(f.human, "UPDATE deal_stage_history SET owner_id_at_change=NULL,pipeline_id_at_change=NULL"); err != nil {
		t.Fatal(err)
	}
	selection := f.selection()
	selection.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"owner_attainment", "target_progress"}
	_, err := f.service.CreateTarget(f.human, crmcontracts.ReportingTargetInput{Metric: "bookings_won", PipelineId: ptrUUID(f.env.pipeline), Scope: selection.Scope, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 10800000, Reason: "Monthly sales target"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	sales := result.Metrics[0]
	if sales.Value == nil || *sales.Value != 10800000 || sales.TargetActual == nil || *sales.TargetActual != 10800000 || sales.Attainment == nil || *sales.Attainment != 100 || sales.Coverage.Status != "ok" {
		t.Fatalf("existing sales/target: %+v", sales)
	}
	rate := result.Metrics[1]
	if rate.Value == nil || *rate.Value != 60 || rate.Coverage.Status != "ok" {
		t.Fatalf("existing win rate: %+v", rate)
	}
	group := "owner:" + f.env.Rep1.String()
	evidence, err := f.service.Evidence(f.human, selection, "bookings_won", "interval", group, nil, nil, 50, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 6 {
		t.Fatalf("expected six wins, got %d", len(evidence.Rows))
	}
	for _, row := range evidence.Rows {
		if row.SourceId == nil || row.OwnerId == nil || ids.UUID(*row.OwnerId) != f.env.Rep1 {
			t.Fatalf("wrong current owner: %+v", row)
		}
		deal, err := f.store.GetDeal(f.human, ids.From[ids.DealKind](ids.UUID(*row.SourceId)), storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if deal.OwnerId == nil || *deal.OwnerId != *row.OwnerId {
			t.Fatalf("report and deal disagree: %+v %+v", row, deal.OwnerId)
		}
	}
}

func TestReportingUnassignedWinDoesNotRetainThePreviousOwner(t *testing.T) {
	e := setupForecast(t)
	closed, store, writer := reportingWrittenClose(t, e)
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(closed.Id)), deals.UpdateDealInput{Clear: []string{"owner_id"}}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment"}}
	result, err := service.Evaluate(reportingActor(e), selection)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, point := range result.Charts[0].Points {
		if point.Key == ids.Nil.String() {
			found = point.Label == "Unassigned" && point.Value != nil && *point.Value == 21600000
		}
		if point.Key == e.Rep1.String() && point.Value != nil && *point.Value != 0 {
			t.Fatal("unassigned win stayed with former owner")
		}
	}
	if !found {
		t.Fatalf("unassigned win missing: %+v", result.Charts[0])
	}
}

func TestReportingOlderHeldMeetingsUseKnownHostAndCustomerLinks(t *testing.T) {
	for _, history := range []string{"missing_attribution", "partial_snapshot", "no_history"} {
		t.Run(history, func(t *testing.T) {
			f := reportingBusiness(t)
			switch history {
			case "missing_attribution":
				if _, err := f.env.owner.Exec(f.human, "UPDATE activity_meeting_history SET host_id_at_change=NULL,customer_eligible_at_change=NULL"); err != nil {
					t.Fatal(err)
				}
			case "partial_snapshot":
				if _, err := f.env.owner.Exec(f.human, "UPDATE activity_meeting_history SET partial_pre_history=true"); err != nil {
					t.Fatal(err)
				}
			case "no_history":
				if _, err := f.env.owner.Exec(f.human, "DELETE FROM activity_meeting_history"); err != nil {
					t.Fatal(err)
				}
			}
			store := activities.NewStore(InstallationDB(f.env.Pool)).WithClock(func() time.Time { return f.at })
			held := "held"
			start := f.at.Add(-time.Hour)
			subject := "Internal meeting"
			host := ids.From[ids.UserKind](f.env.Rep1)
			if _, _, err := store.LogActivity(f.human, activities.LogActivityInput{Kind: "meeting", HostUserID: &host, Subject: &subject, OccurredAt: &start, MeetingStatus: &held, Source: "manual"}); err != nil {
				t.Fatal(err)
			}
			selection := f.selection()
			selection.PipelineId = nil
			selection.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}
			selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held"}
			selection.Blocks = []crmcontracts.ReportingBlockKind{"sdr_outcomes"}
			result, err := f.service.Evaluate(f.human, selection)
			if err != nil {
				t.Fatal(err)
			}
			metric := result.Metrics[0]
			if metric.Value == nil || *metric.Value != 12 || metric.Coverage.Status != "partial" || metric.Coverage.Reason == nil {
				t.Fatalf("existing meetings: %+v", metric)
			}
			evidence, err := f.service.Evidence(f.human, selection, "meetings_held", "interval", "", nil, nil, 50, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
			if err != nil {
				t.Fatal(err)
			}
			if len(evidence.Rows) != 12 {
				t.Fatalf("meeting evidence: %+v", evidence.Rows)
			}
			for _, row := range evidence.Rows {
				if row.OwnerId == nil || ids.UUID(*row.OwnerId) != f.env.Rep1 {
					t.Fatalf("lost recorded host: %+v", row)
				}
			}
		})
	}
}

func TestReportingFutureHeldTransitionDoesNotBorrowCurrentStatus(t *testing.T) {
	f := reportingBusiness(t)
	if _, err := f.env.owner.Exec(f.human, "UPDATE activity_meeting_history SET effective_at=$1", f.at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	selection := f.selection()
	selection.PipelineId = nil
	selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"sdr_outcomes"}
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics[0].Value == nil || *result.Metrics[0].Value != 0 {
		t.Fatalf("future confirmation was counted: %+v", result.Metrics[0])
	}
}

func TestReportingSavedSalesKeepTheirCapturedOwnerAfterReassignment(t *testing.T) {
	e := setupForecast(t)
	reportingWorkerIdentity(t, e)
	closed, store, writer := reportingWrittenClose(t, e)
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	human := reportingActor(e)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment"}}
	report, err := service.CreateReport(human, crmcontracts.ReportingReportInput{Name: "Sales review", Audience: "workspace", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Freeze(human, ids.UUID(report.Id), report.Revision, "before-transfer")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Sweep(writer); err != nil {
		t.Fatal(err)
	}
	completed, err := service.GetExecution(human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("capture: %+v %v", completed, err)
	}
	owner := ids.From[ids.UserKind](e.Rep3)
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(closed.Id)), deals.UpdateDealInput{OwnerID: &owner}); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.EditionEvidence(human, ids.UUID(*completed.EditionId), "bookings_won", "interval", "", nil, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(frozen.Rows) != 1 || frozen.Rows[0].OwnerId == nil || ids.UUID(*frozen.Rows[0].OwnerId) != e.Rep1 {
		t.Fatalf("snapshot owner changed: %+v", frozen.Rows)
	}
	selection.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}
	live, err := service.Evaluate(human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if live.Metrics[0].Value == nil || *live.Metrics[0].Value != 21600000 {
		t.Fatalf("live owner did not change: %+v", live.Metrics)
	}
}

func TestReportingTransferredWinsAndLossesMoveTargetsAndWinRateTogether(t *testing.T) {
	f := reportingBusiness(t)
	for _, transfer := range []struct {
		name  string
		owner ids.UUID
	}{{"Outcome 00", f.env.Rep3}, {"Outcome 13", f.env.Rep1}} {
		var dealID ids.UUID
		var b reportingBindings
		if err := f.env.owner.QueryRow(f.human, "SELECT id FROM deal WHERE name="+b.add(transfer.name), b.values...).Scan(&dealID); err != nil {
			t.Fatal(err)
		}
		owner := ids.From[ids.UserKind](transfer.owner)
		if _, err := f.store.UpdateDeal(f.writer, ids.From[ids.DealKind](dealID), deals.UpdateDealInput{OwnerID: &owner}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		owner        ids.UUID
		actual, rate float64
	}{{f.env.Rep1, 9000000, 50}, {f.env.Rep3, 12600000, 70}} {
		selection := f.selection()
		selection.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(tc.owner)}
		selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate"}
		selection.Blocks = []crmcontracts.ReportingBlockKind{"owner_attainment", "target_progress"}
		if _, err := f.service.CreateTarget(f.human, crmcontracts.ReportingTargetInput{Metric: "bookings_won", PipelineId: ptrUUID(f.env.pipeline), Scope: selection.Scope, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: int64(tc.actual), Reason: "Owner target"}); err != nil {
			t.Fatal(err)
		}
		result, err := f.service.Evaluate(f.human, selection)
		if err != nil {
			t.Fatal(err)
		}
		sales, rate := result.Metrics[0], result.Metrics[1]
		if sales.Value == nil || *sales.Value != tc.actual || sales.TargetActual == nil || *sales.TargetActual != tc.actual || sales.Attainment == nil || *sales.Attainment != 100 || rate.Value == nil || *rate.Value != tc.rate {
			t.Fatalf("transferred outcome mismatch: %+v", result.Metrics)
		}
	}
	selection := f.selection()
	selection.Scope = crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(f.env.Team1)}
	selection.Metrics = []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"metric_reading"}
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	if result.Metrics[0].Value == nil || *result.Metrics[0].Value != 9000000 || result.Metrics[1].Value == nil || *result.Metrics[1].Value != 50 {
		t.Fatalf("team retained transferred outcomes: %+v", result.Metrics)
	}
}
