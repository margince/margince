// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One account, one field name, one number.
//
// `open_pipeline_minor_base` is published twice for the same organization: the
// company RECORD computes it in SQL (organization_open_pipeline_rollup, read by
// people/organization_computed.go) and the company PAGE computes it in Go
// (org360's priceOpenDeals over deals.PriceAll). Two implementations of one
// rule drift, and this one did: the Go side learned to scale both currencies'
// minor units and the SQL side did not, so a yen deal came out a hundredfold
// apart on two screens that name the same figure.
//
// The currency is JPY on purpose. It carries no minor unit where EUR carries
// two, so the two spellings agree for every same-scale pair — EUR, USD, GBP,
// CHF, every pair in the demo data — and only a zero-decimal currency can tell
// them apart. A same-currency fixture here would pass against both the fixed
// and the broken arithmetic.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// TestBothOpenPipelineReadsAgreeOnAYenDeal is the parity case rule 7 asks for:
// one invariant spelled on both sides of a wire is ONE item, and a gate that
// fails in both directions is what keeps it that way.
func TestBothOpenPipelineReadsAgreeOnAYenDeal(t *testing.T) {
	e := Setup(t)
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	orgID := e.SeedOrg(t, "Yen Pipeline KK", nil)

	// ¥5,000,000 — five million yen, since JPY has no minor unit — at
	// 1 JPY = 0.006 EUR is €30,000, which is 3,000,000 EUR minor units.
	// An unscaled multiply answers 30,000: €300.
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Tokyo renewal", AmountMinor: int64Ptr(5_000_000), Currency: strPtr("JPY"),
		PipelineID: pipeline, StageID: open, OrganizationID: orgIDPtr(orgIDOf(orgID)), Source: "manual",
	}); err != nil {
		t.Fatalf("seeding the yen deal: %v", err)
	}
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('JPY', 'EUR', 0.0060000000, DATE '2020-01-01')`)

	const wantMinorBase = int64(3_000_000)

	// The SQL side, as the company record publishes it.
	org, err := e.People.GetOrganization(e.Admin(), orgIDOf(orgID), storekit.IncludeArchived)
	if err != nil {
		t.Fatalf("reading the organization: %v", err)
	}
	record := computedFieldByKey(*org.ComputedFields, "open_pipeline")
	if !record.Computable {
		t.Fatal("the company record reports the pipeline as not computable, with a rate loaded for the pair")
	}
	if record.ValueMinor == nil || *record.ValueMinor != wantMinorBase {
		t.Errorf("the company RECORD says %v, want %d (€30,000); 30000 is the unscaled multiply, which reads "+
			"five million yen as three hundred euros", record.ValueMinor, wantMinorBase)
	}

	// The Go side, as the company page publishes it.
	page, err := orgSurfaceService(e).Assemble(e.Admin(), orgIDOf(orgID))
	if err != nil {
		t.Fatalf("assembling the company page: %v", err)
	}
	if page.StateStrip == nil || page.StateStrip.Commercial == nil {
		t.Fatal("the company page reports no commercial strip for an account holding one open deal")
	}
	strip := page.StateStrip.Commercial
	if strip.OpenPipelineMinorBase == nil {
		t.Fatal("the company PAGE priced nothing, with a rate loaded for the pair")
	}
	if int64(*strip.OpenPipelineMinorBase) != wantMinorBase {
		t.Errorf("the company PAGE says %d, want %d (€30,000)", *strip.OpenPipelineMinorBase, wantMinorBase)
	}

	// And the assertion the two halves above exist for: whatever the number is,
	// both surfaces say it. A test that only checked each against a constant
	// would keep passing if both drifted together, which is a weaker claim than
	// the one this file's subject needs.
	if record.ValueMinor != nil && int64(*strip.OpenPipelineMinorBase) != *record.ValueMinor {
		t.Errorf("the record says %d and the page says %d for the same account — one field name, two amounts",
			*record.ValueMinor, *strip.OpenPipelineMinorBase)
	}
	if strip.BaseCurrency == nil || *strip.BaseCurrency != "EUR" {
		t.Errorf("the page labels the figure %v, want EUR", strip.BaseCurrency)
	}
}
