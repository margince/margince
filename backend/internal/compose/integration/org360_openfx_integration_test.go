// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The company page's open pipeline, when the account's deals are not all held
// in the installation's own currency.
//
// A deal freezes its conversion rate on CLOSE, so `amount_minor_base` is null on
// every open one. The read converts those at the latest stored rate on or before
// its as-of day — otherwise a pipeline held in USD prices at nothing on a EUR
// installation, forever, and the page reports an empty total for an account that
// plainly has deals in it.
//
// What must NOT happen is the other failure: a currency with no rate at all
// contributing zero, or contributing at a rate of 1. Such a deal stays counted
// and unpriced, so the total covers part of the pipeline rather than being
// silently short.

import (
	"testing"
	"time"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// openFXStages is one pipeline with one open stage, which is all these cases
// need — they turn on money rather than on stage shape.
type openFXStages struct {
	pipeline ids.UUID
	open     ids.UUID
}

func seedOpenFXPipeline(t *testing.T, e *Env) openFXStages {
	t.Helper()
	st := openFXStages{pipeline: ids.NewV7(), open: ids.NewV7()}
	e.WsExec(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Open FX')`, st.pipeline)
	e.WsExec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualified', 1, 'open', 0.5)`, st.open, st.pipeline)
	return st
}

func seedOpenFXDeal(t *testing.T, e *Env, st openFXStages, org ids.UUID, amountMinor int64, currency string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO deal (id, name, amount_minor, currency, pipeline_id, stage_id, organization_id, status, source, captured_by)
		VALUES ($1, 'Open FX Deal', $2, $3, $4, $5, $6, 'open', 'manual', 'human:test')`,
		ids.NewV7(), amountMinor, currency, st.pipeline, st.open, org)
}

func TestAnOpenDealInAnotherCurrencyIsPricedAtTheLatestRate(t *testing.T) {
	e := Setup(t)
	st := seedOpenFXPipeline(t, e)
	org := ids.NewV7()
	e.WsExec(t, `INSERT INTO organization (id, display_name, lifecycle, source, captured_by)
		VALUES ($1, 'Mixed Pipeline GmbH', 'customer', 'manual', 'human:test')`, org)

	// One deal in the base currency and one in USD, with a rate loaded for the
	// pair. 100.00 EUR + (200.00 USD × 0.9) = 280.00 EUR.
	seedOpenFXDeal(t, e, st, org, 10_000, "EUR")
	seedOpenFXDeal(t, e, st, org, 20_000, "USD")
	// Dated well before any clock this suite runs under: the read asks for the
	// latest rate on or BEFORE its as-of day, and the harness pins that day
	// rather than reading the wall clock.
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('USD', 'EUR', 0.9, DATE '2020-01-01')`)

	page, err := orgSurfaceService(e).Assemble(e.Admin(), orgIDOf(org))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.StateStrip == nil || page.StateStrip.Commercial == nil {
		t.Fatal("the company page reports no commercial strip for an account holding two open deals")
	}
	got := page.StateStrip.Commercial
	if got.OpenCount != 2 {
		t.Errorf("open_count = %d, want 2", got.OpenCount)
	}
	if got.PricedCount != 2 {
		t.Errorf("priced = %d, want 2 — the USD deal converts at the loaded rate", got.PricedCount)
	}
	if got.OpenPipelineMinorBase == nil {
		t.Error("open_pipeline_minor_base is absent; the USD deal did not convert")
	} else if *got.OpenPipelineMinorBase != 28_000 {
		t.Errorf("open_pipeline_minor_base = %d, want 28000 (100.00 EUR + 200.00 USD at 0.9)", *got.OpenPipelineMinorBase)
	}
	// A converted total says what it converted to and when.
	if got.BaseCurrency == nil || *got.BaseCurrency != "EUR" {
		t.Errorf("base_currency = %v, want EUR — a converted sum wearing no label is the unlabelled cross-currency total", got.BaseCurrency)
	}
	if got.FxAsOf == nil {
		t.Error("no fx_as_of on a converted total; a converted figure carries the date its rate was read from")
	}
}

func TestAnOpenDealInACurrencyWithNoRateStaysCountedAndUnpriced(t *testing.T) {
	e := Setup(t)
	st := seedOpenFXPipeline(t, e)
	org := ids.NewV7()
	e.WsExec(t, `INSERT INTO organization (id, display_name, lifecycle, source, captured_by)
		VALUES ($1, 'Unrated Pipeline GmbH', 'customer', 'manual', 'human:test')`, org)

	// No fx_rate row for JPY at all. Inventing a rate — or treating the
	// missing one as 1 — would report ¥50,000 as €50,000.
	seedOpenFXDeal(t, e, st, org, 10_000, "EUR")
	seedOpenFXDeal(t, e, st, org, 5_000_000, "JPY")

	page, err := orgSurfaceService(e).Assemble(e.Admin(), orgIDOf(org))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.StateStrip == nil || page.StateStrip.Commercial == nil {
		t.Fatal("the company page reports no commercial strip")
	}
	got := page.StateStrip.Commercial
	if got.OpenCount != 2 {
		t.Errorf("open_count = %d, want 2 — an unpriceable deal is still a deal in the pipeline", got.OpenCount)
	}
	if got.PricedCount != 1 {
		t.Errorf("priced = %d, want 1 — only the EUR deal can be priced", got.PricedCount)
	}
	if got.OpenPipelineMinorBase == nil {
		t.Error("open_pipeline_minor_base is absent; the EUR deal alone should still price")
	} else if *got.OpenPipelineMinorBase != 10_000 {
		t.Errorf("open_pipeline_minor_base = %d, want 10000 — the JPY deal contributes nothing rather than contributing wrongly", *got.OpenPipelineMinorBase)
	}
}

func TestTheLatestRateOnOrBeforeTheAsOfDayIsTheOneUsed(t *testing.T) {
	e := Setup(t)
	st := seedOpenFXPipeline(t, e)
	org := ids.NewV7()
	e.WsExec(t, `INSERT INTO organization (id, display_name, lifecycle, source, captured_by)
		VALUES ($1, 'Rate Ladder GmbH', 'customer', 'manual', 'human:test')`, org)
	seedOpenFXDeal(t, e, st, org, 20_000, "USD")

	// Three rates: an older one, the one that should win, and one dated AFTER
	// the read's as-of day. Picking the newest row outright would take the
	// future rate; picking the oldest would take 0.7. Only "the latest on or
	// before the as-of day" gives 0.9, and only a ladder like this can tell the
	// three apart.
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date) VALUES
		('USD', 'EUR', 0.7, DATE '2020-01-01'),
		('USD', 'EUR', 0.9, DATE '2020-06-01'),
		('USD', 'EUR', 0.2, DATE '2099-01-01')`)

	page, err := orgSurfaceService(e).Assemble(e.Admin(), orgIDOf(org))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	got := page.StateStrip.Commercial
	if got.OpenPipelineMinorBase == nil {
		t.Fatal("open_pipeline_minor_base is absent; the USD deal did not convert")
	}
	if *got.OpenPipelineMinorBase != 18_000 {
		t.Errorf("open_pipeline_minor_base = %d, want 18000 (200.00 USD at 0.9) — 14000 means the oldest rate won, 4000 means a future rate did",
			*got.OpenPipelineMinorBase)
	}
	// The date reported is the date of the rate actually applied, not some
	// other rate's day: a figure whose as-of names a rate it was not computed
	// at is the unlabelled cross-currency total wearing a label.
	if got.FxAsOf == nil {
		t.Fatal("no fx_as_of on a converted total")
	}
	if want := "2020-06-01"; got.FxAsOf.Format(time.DateOnly) != want {
		t.Errorf("fx_as_of = %s, want %s — the date must name the rate the amount was computed at",
			got.FxAsOf.Format(time.DateOnly), want)
	}
}

func TestAStaleRateDateOnAnOpenDealDoesNotBecomeTheAsOf(t *testing.T) {
	e := Setup(t)
	st := seedOpenFXPipeline(t, e)
	org := ids.NewV7()
	e.WsExec(t, `INSERT INTO organization (id, display_name, lifecycle, source, captured_by)
		VALUES ($1, 'Stale Date GmbH', 'customer', 'manual', 'human:test')`, org)

	// An open deal may carry fx_rate_date with no fx_rate_to_base beside it —
	// the schema constrains the pair only for a CLOSED deal (deal_closed_fx).
	// That stored date describes no rate that has been applied to anything, so
	// reporting it as the as-of would name a day whose rate is not the rate the
	// figure was computed at.
	e.WsExec(t, `INSERT INTO deal (id, name, amount_minor, currency, fx_rate_date,
		         pipeline_id, stage_id, organization_id, status, source, captured_by)
		VALUES ($1, 'Stale Date Deal', 20000, 'USD', DATE '2019-01-01', $2, $3, $4, 'open', 'manual', 'human:test')`,
		ids.NewV7(), st.pipeline, st.open, org)
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('USD', 'EUR', 0.9, DATE '2020-06-01')`)

	page, err := orgSurfaceService(e).Assemble(e.Admin(), orgIDOf(org))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	got := page.StateStrip.Commercial
	if got.OpenPipelineMinorBase == nil || *got.OpenPipelineMinorBase != 18_000 {
		t.Fatalf("open_pipeline_minor_base = %v, want 18000 — the live rate is what priced it", got.OpenPipelineMinorBase)
	}
	if got.FxAsOf == nil {
		t.Fatal("no fx_as_of on a converted total")
	}
	if want := "2020-06-01"; got.FxAsOf.Format(time.DateOnly) != want {
		t.Errorf("fx_as_of = %s, want %s — the deal's own stale fx_rate_date describes no applied rate and must not be reported",
			got.FxAsOf.Format(time.DateOnly), want)
	}
}
