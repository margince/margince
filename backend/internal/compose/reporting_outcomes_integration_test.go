// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReportingQualifiedPipelineKeepsTheFirstEntryThroughRegression(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return at }
	ctx := reportingActor(e)
	service := newReportingService(e.Pool, clock)
	qualification := crmcontracts.ReportingQualification{PipelineId: openapi_types.UUID(e.pipeline), StageIds: []openapi_types.UUID{openapi_types.UUID(e.stages[60])}}
	if _, err := service.PublishFramework(ctx, 0, crmcontracts.ReportingFrameworkInput{Template: "sales", Reason: "Qualified on proposal", Qualification: []crmcontracts.ReportingQualification{qualification}, CaptureContexts: []crmcontracts.ReportingCaptureContext{}}); err != nil {
		t.Fatal(err)
	}
	writer := principal.SystemActing(ctx, "system:qualification-fixture")
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(clock)
	owner := ids.From[ids.UserKind](e.Rep1)
	amount, currency := int64(10000), "EUR"
	at = at.AddDate(0, 0, 1)
	deal, err := store.CreateDeal(writer, deals.CreateDealInput{Name: "Qualified once", AmountMinor: &amount, Currency: &currency, PipelineID: ids.From[ids.PipelineKind](e.pipeline), StageID: ids.From[ids.StageKind](e.stages[20]), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []ids.UUID{e.stages[60], e.stages[20], e.stages[60]} {
		at = at.Add(time.Hour)
		if _, err := store.AdvanceDeal(writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](stage)}); err != nil {
			t.Fatal(err)
		}
	}
	owner = ids.From[ids.UserKind](e.Rep3)
	amount = 25000
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.UpdateDealInput{OwnerID: &owner, AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	at = at.Add(time.Hour)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", PipelineId: ptrUUID(e.pipeline), Metrics: []crmcontracts.ReportingMetricID{"qualified_pipeline_created"}, Blocks: []crmcontracts.ReportingBlockKind{"metric_reading"}}
	result, err := service.Evaluate(ctx, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Value == nil || *result.Metrics[0].Value != 10000 {
		t.Fatalf("qualification credit was duplicated or restated: %+v", result.Metrics)
	}
}

func TestReportingAcceptedCreditSurvivesTransferAndDeduplicatesHandoffs(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return at }
	ctx := reportingActor(e)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	contact := e.seedID(t, `INSERT INTO contact(id,first_name,last_name,full_name,source,captured_by) VALUES($1,'Jamie','Buyer','Jamie Buyer','manual','system:fixture')`)
	deal, store, writer := reportingWrittenClose(t, e)
	contactsStore := contacts.NewStore(InstallationDB(e.Pool)).WithClock(clock)
	for range 2 {
		handoff, err := contactsStore.SubmitHandoff(ctx, contacts.NewSDRHandoff{ContactID: &contact})
		if err != nil {
			t.Fatal(err)
		}
		dealID := ids.UUID(deal.Id)
		if err := contactsStore.DecideHandoff(ctx, handoff, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID}); err != nil {
			t.Fatal(err)
		}
		at = at.Add(time.Hour)
	}
	owner := ids.From[ids.UserKind](e.Rep3)
	if _, err := store.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), deals.UpdateDealInput{OwnerID: &owner}); err != nil {
		t.Fatal(err)
	}
	// Credit remains readable without source contact/deal grants.
	actor.Permissions.Objects = map[string]principal.ObjectGrant{"reporting_credit": {Read: true}, "report_definition": {Read: true}, "installation_settings": {Read: true}}
	reader := principal.WithActor(ctx, actor)
	service := newReportingService(e.Pool, clock)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"accepted_opportunities"}, Blocks: []crmcontracts.ReportingBlockKind{"sdr_outcomes"}}
	result, err := service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Value == nil || *result.Metrics[0].Value != 1 {
		t.Fatalf("duplicate/lost acceptance credit: %+v", result.Metrics)
	}
	evidence, err := service.Evidence(reader, selection, "accepted_opportunities", "interval", "", nil, nil, 50, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 || !evidence.Rows[0].Restricted || evidence.Rows[0].SourceId != nil {
		t.Fatalf("credit disclosed its restricted source: %+v", evidence.Rows)
	}
}
