// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package initiativeenrolment_test

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// manufacturingLead creates a lead in the manufacturing industry through the
// contacts store.
func (w manufacturingDeals) manufacturingLead(t *testing.T, email string) ids.UUID {
	t.Helper()
	company := "Linden Works"
	lead, _, err := w.e.Contacts.CreateLead(w.e.Admin(), contacts.CreateLeadInput{Email: &email, CompanyName: &company, Source: "manual"})
	if err != nil {
		t.Fatalf("creating the lead %s: %v", email, err)
	}
	id := ids.UUID(lead.Id)
	w.e.WsExec(t, `UPDATE lead SET industry = 'manufacturing' WHERE id = $1`, id)
	return id
}

// promote qualifies the lead into a new deal through the agent door compose
// builds, the path that archives the lead, and answers the deal.
func (w manufacturingDeals) promote(t *testing.T, lead ids.UUID, amount int64) ids.UUID {
	t.Helper()
	currency := "EUR"
	choice := datasource.PromotionDealChoice{Deal: &datasource.PromotionNewDeal{AmountMinor: &amount, Currency: &currency}}
	if _, _, err := compose.NewProvider(w.e.Pool).PromoteLead(w.e.Admin(), lead, "human_qualify", nil, nil, choice); err != nil {
		t.Fatalf("promoting the lead: %v", err)
	}
	deal, err := ids.Parse(w.e.WsScalar(t, `SELECT qualified_deal_id::text FROM lead WHERE id = $1 AND archived_at IS NOT NULL`, lead))
	if err != nil {
		t.Fatalf("the promoted lead names no deal: %v", err)
	}
	w.e.WsExec(t, `UPDATE deal SET amount_minor = $2, currency = 'EUR' WHERE id = $1`, deal, amount)
	return deal
}

// A deal is sourced when its lead was enrolled at the promotion. That holds
// for a lead the launch enrolled and for one created after the launch and
// promoted before any sweep saw it. A lead taken out before its promotion
// sourced nothing, even with its deal picked later.
func TestALeadEnrolledAtItsPromotionSourcedItsDeal(t *testing.T) {
	w := seedManufacturingDeals(t)
	ctx := w.manager()
	early := w.manufacturingLead(t, "early@linden.test")
	leaver := w.manufacturingLead(t, "leaver@linden.test")
	rule, future := storekit.Predicate{Field: "industry", Op: "eq", Value: "manufacturing"}, true
	draft, err := w.store.Create(ctx, initiatives.Definition{Name: "Lead push", Objective: "qualified_opportunities_created",
		Deadline: clocktest.Now(t).UTC().AddDate(0, 3, 0), LeadRule: &rule, IncludeFuture: &future})
	if err != nil {
		t.Fatalf("defining the initiative: %v", err)
	}
	id := ids.UUID(draft.Id)
	if _, err := w.store.Launch(ctx, id, nil); err != nil {
		t.Fatalf("launching: %v", err)
	}
	if _, err := w.store.ReplaceMembers(ctx, id, nil, refs("lead", leaver)); err != nil {
		t.Fatalf("taking the leaver out: %v", err)
	}

	fresh := w.manufacturingLead(t, "fresh@linden.test")
	w.promote(t, early, 10_000)
	w.promote(t, fresh, 20_000)
	leaverDeal := w.promote(t, leaver, 4_000)
	if _, err := w.store.ReplaceMembers(ctx, id, refs("deal", leaverDeal), nil); err != nil {
		t.Fatalf("picking the leaver's deal: %v", err)
	}

	now := clocktest.Now(t).UTC()
	service := compose.NewReportingService(w.e.Pool, func() time.Time { return now })
	interval := crmcontracts.ReportingWindow{StartAt: now.AddDate(0, 0, -3), EndAt: now.AddDate(0, 0, 3)}
	wire := openapi_types.UUID(id)
	result, err := service.Evaluate(ctx, crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open",
		InitiativeId: &wire, Metrics: []crmcontracts.ReportingMetricID{initiativePipeline}, Blocks: []crmcontracts.ReportingBlockKind{"breakdown"},
	})
	if err != nil {
		t.Fatalf("evaluating under the initiative: %v", err)
	}
	if origins := originSplit(t, result); origins["origin:sourced"] != 30_000 || origins["origin:created_after"] != 4_000 {
		t.Fatalf("the origin split is %v, want the early and fresh leads' deals sourced and the leaver's created after", origins)
	}
}

const initiativePipeline crmcontracts.ReportingMetricID = "initiative_pipeline"

// originSplit reads the initiative pipeline's breakdown by origin.
func originSplit(t *testing.T, result crmcontracts.ReportingEvaluation) map[string]float64 {
	t.Helper()
	for _, chart := range result.Charts {
		if chart.Kind != "breakdown" || chart.Metric != initiativePipeline {
			continue
		}
		values := map[string]float64{}
		for _, point := range chart.Points {
			if point.Value != nil {
				values[point.Key] = *point.Value
			}
		}
		return values
	}
	t.Fatal("the initiative pipeline has no breakdown chart")
	return nil
}
