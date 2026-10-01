// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func reportingActor(e *forecastEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"report_definition", "report_edition", "report_schedule", "reporting_framework", "sales_target", "deal", "pipeline", "installation_settings", "forecast", "reporting_credit", "activity", "company", "contact", "team_oversight"} {
		grants[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull, TeamIDs: []ids.UUID{e.Team1}, Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants}})
}

func reportingWrittenClose(t *testing.T, e *forecastEnv) (crmcontracts.Deal, *deals.Store, context.Context) {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:reporting-fixture")
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	won := e.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Won',10,'won',100)`, e.pipeline)
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC) })
	owner := ids.From[ids.UserKind](e.Rep1)
	amount := int64(21600000)
	currency := "EUR"
	created, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Booked through the production writer", AmountMinor: &amount, Currency: &currency, PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	reason := "purchase_order"
	closed, err := store.AdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(created.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](won), WonWithoutContractReason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	return closed, store, ctx
}

func TestReportingBookingsFollowCurrentOwnerAfterTransfer(t *testing.T) {
	e := setupForecast(t)
	closed, store, writer := reportingWrittenClose(t, e)
	if closed.ClosedAt == nil {
		t.Fatal("production writer returned no close timestamp")
	}
	owner := ids.From[ids.UserKind](e.Rep3)
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(closed.Id)), deals.UpdateDealInput{OwnerID: &owner}); err != nil {
		t.Fatal(err)
	}
	at := closed.ClosedAt.Add(2 * time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return at })
	interval := crmcontracts.ReportingWindow{StartAt: closed.ClosedAt.Add(-time.Hour), EndAt: closed.ClosedAt.Add(time.Hour)}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "owner_attainment"}}
	result, err := service.Evaluate(reportingActor(e), selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Value == nil || *result.Metrics[0].Value != 21600000 {
		t.Fatalf("sales did not follow current ownership: %+v", result.Metrics)
	}
	evidence, err := service.Evidence(reportingActor(e), selection, "bookings_won", "interval", "", nil, nil, 50, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 || evidence.Rows[0].SourceId == nil || *evidence.Rows[0].SourceId != closed.Id || evidence.Rows[0].OwnerId == nil || ids.UUID(*evidence.Rows[0].OwnerId) != e.Rep3 {
		t.Fatalf("wrong current-owner evidence: %+v", evidence.Rows)
	}
	selection.Scope.Id = ptrUUID(e.Rep1)
	other, err := service.Evaluate(reportingActor(e), selection)
	if err != nil {
		t.Fatal(err)
	}
	if other.Metrics[0].Value == nil || *other.Metrics[0].Value != 0 {
		t.Fatalf("previous owner retained reassigned sales: %+v", other.Metrics)
	}
	amount := int64(9900000)
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(closed.Id)), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	selection.Scope.Id = ptrUUID(e.Rep3)
	_, err = service.Evidence(reportingActor(e), selection, "bookings_won", "interval", "", nil, nil, 50, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("changed evidence must refresh the reading: %v", err)
	}
}

func TestReportingSavedLayoutRetainsEverySelectedBlock(t *testing.T) {
	e := setupForecast(t)
	ctx := reportingActor(e)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "fiscal_quarter", PipelineId: ptrUUID(e.pipeline), Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "open_pipeline", "stage_age"}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution", "owner_attainment", "stage_age"}}
	created, err := service.CreateReport(ctx, crmcontracts.ReportingReportInput{Name: "My operating view", Audience: "private", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	read, err := service.GetReport(ctx, ids.UUID(created.Id))
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Selection.Blocks) != 4 || read.Selection.Blocks[3] != "stage_age" || read.Selection.PipelineId == nil || ids.UUID(*read.Selection.PipelineId) != e.pipeline {
		t.Fatalf("saved layout changed: %+v", read.Selection)
	}
}
