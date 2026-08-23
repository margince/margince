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

func TestAClosedDealKeepsTheRateItFroze(t *testing.T) {
	e := Setup(t)
	st := seedOpenFXPipeline(t, e)
	org := ids.NewV7()
	e.WsExec(t, `INSERT INTO organization (id, display_name, lifecycle, source, captured_by)
		VALUES ($1, 'Frozen Rate GmbH', 'customer', 'manual', 'human:test')`, org)

	// A deal that froze at 0.8 while the rate sheet now says 0.5. A READ must
	// not re-price it: the figure it closed at is the figure it closed at, and
	// re-converting history on every page load is exactly what freezing exists
	// to prevent.
	closedAt := time.Date(2020, 6, 1, 12, 0, 0, 0, time.UTC)
	e.WsExec(t, `INSERT INTO deal (id, name, amount_minor, currency, fx_rate_to_base, fx_rate_date,
		         pipeline_id, stage_id, organization_id, status, closed_at, source, captured_by)
		VALUES ($1, 'Frozen Deal', 20000, 'USD', 0.8, $2::date, $3, $4, $5, 'won', $6, 'manual', 'human:test')`,
		ids.NewV7(), closedAt, st.pipeline, st.open, org, closedAt)
	e.WsExec(t, `INSERT INTO fx_rate (from_currency, to_currency, rate, rate_date)
		VALUES ('USD', 'EUR', 0.5, DATE '2020-02-01')`)

	stored := e.WsCount(t, `SELECT amount_minor_base FROM deal WHERE organization_id = $1`, org)
	if stored != 16_000 {
		t.Errorf("the closed deal's stored base amount = %d, want 16000 (200.00 USD at the 0.8 it froze) — a later rate must not reach it", stored)
	}
}
