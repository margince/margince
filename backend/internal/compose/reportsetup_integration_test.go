// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// The model reads the live products and the reader's metrics. A product it
// names by its real id is resolved, and one it invents is named back. The
// proposal writes nothing: the initiative list stays empty.
func TestTheSetupProposalResolvesTheCatalogTheReaderMaySee(t *testing.T) {
	e := setupForecast(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	ctx := initiativeManager(e)
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	linden, err := store.CreateProduct(ctx, deals.CreateProductInput{Name: "Linden press line", Currency: "EUR", UnitPriceMinor: 250000, Source: "manual"})
	if err != nil {
		t.Fatalf("seeding the product: %v", err)
	}
	service := newReportingService(e.Pool, func() time.Time { return start })
	lane := &assessmentLane{reply: `{"name":"Linden push","filters":[{"field":"product","value":"` + linden.Id.String() + `","phrase":"Linden press"},{"field":"product","value":"01a19fad-0000-7000-8000-000000000000","phrase":"the other line"},{"field":"country","value":"de","phrase":"Germany"}],"metrics":[],"charts":[],"objective":{"metric":"bookings_won","target_value":250000,"deadline":"2099-12-31","phrase":"250k by year end"},"clarifications":["Which quarter?"],"unsupported":[]}`}
	setups := newReportSetups(service, store, e.Pool).withLane(lane)
	out, err := setups.Propose(ctx, crmcontracts.ReportSetupRequest{Site: crmcontracts.ReportSetupRequestSiteInitiativeSetup, Description: "Push the Linden press line in Germany"})
	if err != nil {
		t.Fatalf("the proposal: %v", err)
	}
	if out.Selection.ProductId == nil || out.Selection.ProductId.String() != linden.Id.String() || out.Selection.Country == nil || string(*out.Selection.Country) != "DE" {
		t.Fatalf("the selection is %+v, want the seeded product and DE", out.Selection)
	}
	if out.Objective == nil || out.Objective.Metric != reportingBookingsWon || out.Objective.TargetValue == nil || *out.Objective.TargetValue != 250000 {
		t.Fatalf("the objective is %+v", out.Objective)
	}
	if len(out.Unsupported) != 1 || out.Unsupported[0].Code != crmcontracts.ReportSetupUnsupportedCodeUnknownValue || len(out.Clarifications) != 1 {
		t.Fatalf("named back %+v with clarifications %v, want the invented product alone", out.Unsupported, out.Clarifications)
	}
	if lane.calls != 1 {
		t.Fatalf("the lane was asked %d times", lane.calls)
	}
	listed, err := newInitiativeStore(e.Pool).List(ctx, nil)
	if err != nil {
		t.Fatalf("listing initiatives: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("a proposal wrote %d initiatives", len(listed))
	}
}

func TestTheSetupProposalRefusesWithoutALane(t *testing.T) {
	e := setupForecast(t)
	ctx := initiativeManager(e)
	setups := newReportSetups(newReportingService(e.Pool, time.Now), deals.NewStore(InstallationDB(e.Pool), DealsInstallation()), e.Pool)
	if _, err := setups.Propose(ctx, crmcontracts.ReportSetupRequest{Site: crmcontracts.ReportSetupRequestSiteCustomReportSetup, Description: "Sales this month"}); err == nil {
		t.Fatal("a proposal without a lane answered")
	}
}
