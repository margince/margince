// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingMovementUsesTheDailyTeamPipelineProducer(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { return at }
	service := newReportingService(e.Pool, now)
	reader := reportingActor(e)
	capture := crmcontracts.ReportingCaptureContext{Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, PipelineId: ptrUUID(e.pipeline)}
	_, err := service.PublishFramework(reader, 0, crmcontracts.ReportingFrameworkInput{Template: "sales", Reason: "Capture the team operating view", Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: []crmcontracts.ReportingCaptureContext{capture}})
	if err != nil {
		t.Fatal(err)
	}
	writer := principal.WithCorrelationID(principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:movement-fixture"), ids.NewV7())
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(now)
	owner := ids.From[ids.UserKind](e.Rep1)
	closeDate := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	amount, currency := int64(10000), "EUR"
	deal, err := store.CreateDeal(writer, deals.CreateDealInput{Name: "Movement source", AmountMinor: &amount, Currency: &currency, ExpectedClose: &closeDate, PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	worker := &forecastSnapshotSweepWorker{reportingEnabled: true, pool: e.Pool, now: now, log: slog.New(slog.DiscardHandler)}
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	selection := crmcontracts.ReportingSelection{Scope: capture.Scope, PipelineId: capture.PipelineId, Period: "this_month", TargetBasis: "month", CloseWindow: "fiscal_quarter", Metrics: []crmcontracts.ReportingMetricID{"open_pipeline"}, Blocks: []crmcontracts.ReportingBlockKind{"pipeline_movement"}}
	cold, err := service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(cold.Charts) != 1 || cold.Charts[0].Coverage.Status != "unavailable" {
		t.Fatalf("first capture invented a boundary: %+v", cold.Charts)
	}

	if cold.Charts[0].CaptureStatus == nil || cold.Charts[0].CaptureStatus.LastSuccessAt == nil || !cold.Charts[0].CaptureStatus.LastSuccessAt.Equal(at) {
		t.Fatalf("missing capture status: %+v", cold.Charts[0].CaptureStatus)
	}
	at = at.AddDate(0, 0, 7)
	amount = 13000
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	capturedAt := at
	at = at.Add(time.Hour)
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("duplicate capture: %v", err)
	}
	result, err := service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	chart := result.Charts[0]
	if chart.CaptureStatus == nil || chart.CaptureStatus.LastSuccessAt == nil || !chart.CaptureStatus.LastSuccessAt.Equal(capturedAt) || !chart.CaptureStatus.LastAttemptAt.Equal(at) {
		t.Fatalf("duplicate attempt changed the capture timestamp: %+v", chart.CaptureStatus)
	}
	if chart.Opening == nil || chart.Closing == nil || *chart.Opening != 10000 || *chart.Closing != 13000 {
		t.Fatalf("worker boundaries: %+v", chart)
	}
	total := *chart.Opening
	for _, point := range chart.Points {
		if point.Value != nil {
			total += *point.Value
		}
	}
	if total != *chart.Closing {
		t.Fatalf("movement does not reconcile: %v -> %v", total, *chart.Closing)
	}
	evidence, err := service.Evidence(reader, selection, "open_pipeline", "movement_delta", "amount", nil, nil, 50, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 || evidence.Rows[0].Value == nil || *evidence.Rows[0].Value != 3000 {
		t.Fatalf("movement evidence: %+v", evidence.Rows)
	}
	selection.Scope.Id = ptrUUID(e.Team2)
	other, err := service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	if other.Charts[0].Opening != nil {
		t.Fatal("another team borrowed these snapshots")
	}
	selection.Scope = capture.Scope
	at = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := worker.snapshotWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}
	quarter, err := service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	if quarter.Charts[0].Opening != nil || quarter.Charts[0].Coverage.Status != "unavailable" {
		t.Fatal("new quarter borrowed a prior-quarter boundary")
	}
}
