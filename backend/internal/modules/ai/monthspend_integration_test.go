// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// The month's spend figure against the usage report it summarises. The summary
// must equal the sum of the report's cost_est_minor lines, both read over the
// wire at the same instant.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	spendPricedModel   = "spend-priced"
	spendUnpricedModel = "spend-unpriced"
)

// spendLedger writes usage, calls and rates through the writers production
// uses, each at a chosen instant. CallMeter stamps occurred_at from the
// database clock, so a call's time is moved afterwards on the owner connection.
type spendLedger struct {
	t     *testing.T
	env   *rateEnv
	ws    ids.UUID
	ctx   context.Context
	clock time.Time
	meter *Meter
	calls *CallMeter
	rates *RateStore
}

func newSpendLedger(t *testing.T, env *rateEnv) *spendLedger {
	ws, ctx := env.seedWorkspace(context.Background(), t)
	db := env.dbFor(ws)
	l := &spendLedger{t: t, env: env, ws: ws, ctx: ctx, calls: NewCallMeter(db)}
	l.meter = &Meter{db: db, now: func() time.Time { return l.clock }}
	l.rates = NewRateStore(db).WithClock(func() time.Time { return l.clock })
	return l
}

// rate prices spendPricedModel from the ledger's current day on, at the same
// USD per MTok for input and output.
func (l *spendLedger) rate(usdPerMTok string) {
	l.t.Helper()
	if _, err := l.rates.SetModelRate(laneWriterCtx(l.ws), SetModelRateInput{
		Provider: providerOpenAICompatible, ModelID: spendPricedModel,
		InputUsd: usdPerMTok, OutputUsd: usdPerMTok, CacheReadUsd: "0", CacheWriteUsd: "0",
		EffectiveDate: l.clock,
	}); err != nil {
		l.t.Fatalf("pricing %s at %s: %v", spendPricedModel, usdPerMTok, err)
	}
}

// call meters one terminal call at the ledger's current instant.
func (l *spendLedger) call(task Task, tier Tier, model string, tokensIn, tokensOut int, cacheHit bool) {
	l.t.Helper()
	fingerprint := "fp-" + ids.NewV7().String()
	if err := l.calls.Record(l.ctx, []Call{{
		LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true, Kind: callKindCompletion,
		Task: task, Tier: tier, Provider: providerOpenAICompatible, ModelID: model,
		RequestFingerprint: fingerprint, TokensIn: tokensIn, TokensOut: tokensOut, CacheHit: cacheHit,
	}}); err != nil {
		l.t.Fatalf("recording a call: %v", err)
	}
	if _, err := l.env.owner.Exec(context.Background(),
		`UPDATE ai_call SET occurred_at = $1 WHERE request_fingerprint = $2`, l.clock, fingerprint); err != nil {
		l.t.Fatalf("dating the call: %v", err)
	}
	if err := l.meter.Record(l.ctx, Usage{Task: task, Tier: tier, TokensIn: tokensIn, TokensOut: tokensOut, Cached: cacheHit}); err != nil {
		l.t.Fatalf("metering a call: %v", err)
	}
}

func (l *spendLedger) at(when time.Time) *spendLedger {
	l.clock = when
	return l
}

func (l *spendLedger) handlers() Handlers {
	h := NewHandlers(l.env.dbFor(l.ws), StaticBudget(100_000_000))
	h.meter, h.rates = l.meter, l.rates
	return h
}

// serveSpend runs one handler as reader and decodes its 200 body into out.
func serveSpend[Body crmcontracts.AiUsage | crmcontracts.AiUsageSummary](reader context.Context, t *testing.T, serve http.HandlerFunc, out *Body) {
	t.Helper()
	rec := httptest.NewRecorder()
	serve(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(reader))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if err := json.NewDecoder(rec.Body).Decode(out); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
}

// reportLineSum is what the shell computed from /ai/usage: the sum of the
// lines that carry a cost, or nil when none does.
func reportLineSum(reader context.Context, t *testing.T, h Handlers) *int {
	t.Helper()
	var report crmcontracts.AiUsage
	serveSpend(reader, t, func(w http.ResponseWriter, r *http.Request) {
		h.GetAiUsage(w, r, crmcontracts.GetAiUsageParams{})
	}, &report)
	var sum *int
	for _, day := range report.Days {
		for _, line := range day.Tasks {
			if line.CostEstMinor != nil {
				total := *line.CostEstMinor
				if sum != nil {
					total += *sum
				}
				sum = &total
			}
		}
	}
	return sum
}

func readSummary(reader context.Context, t *testing.T, h Handlers) crmcontracts.AiUsageSummary {
	t.Helper()
	var summary crmcontracts.AiUsageSummary
	serveSpend(reader, t, h.GetAiUsageSummary, &summary)
	return summary
}

func minorText(minor *int) string {
	if minor == nil {
		return "nothing"
	}
	return strconv.Itoa(*minor)
}

func julyDay(day, hour int) time.Time {
	return time.Date(2026, 7, day, hour, 0, 0, 0, time.UTC)
}

// The fixture's pricing: $1 per MTok in and out from June, $2 from 15 July. On
// the $1 rate a call costs its token count in micro-USD, so a cent is 10,000
// tokens. The expected 7 cents is the sum of each line truncated on its own.
// Truncating each call instead gives 6, and pricing the month in one go gives
// 8, so the figure tells the three apart.
func TestMonthSpendEqualsTheSumOfTheReportsPricedLines(t *testing.T) {
	ledger := newSpendLedger(t, setupRateStore(t))
	ledger.at(time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)).rate("1")
	// Last month, and worth 10 cents: the window starts on 1 July.
	ledger.at(time.Date(2026, 6, 30, 23, 0, 0, 0, time.UTC)).call(TaskSummarize, TierCheapCloud, spendPricedModel, 80_000, 20_000, false)

	// Two calls of 6,000 µUSD: 0 cents each, 1 cent as a line.
	ledger.at(julyDay(10, 9)).call(TaskSummarize, TierCheapCloud, spendPricedModel, 5_000, 1_000, false)
	ledger.at(julyDay(10, 10)).call(TaskSummarize, TierCheapCloud, spendPricedModel, 5_000, 1_000, false)
	// 9,999 µUSD: a priced line worth 0 cents, still present.
	ledger.at(julyDay(10, 11)).call(TaskSummarize, TierPremium, spendPricedModel, 8_999, 1_000, false)
	// 15,000 µUSD beside an unpriced call: a partial line of 1 cent.
	ledger.at(julyDay(11, 9)).call(TaskEnrich, TierCheapCloud, spendPricedModel, 10_000, 5_000, false)
	ledger.at(julyDay(11, 10)).call(TaskEnrich, TierCheapCloud, spendUnpricedModel, 2_000, 0, false)
	// Only unpriced calls: the line carries no cost.
	ledger.at(julyDay(11, 11)).call(TaskEnrich, TierPremium, spendUnpricedModel, 3_000, 1_000, false)
	// A call that reached no provider is free, not unpriced: a 0-cent line.
	ledger.at(julyDay(12, 9)).call(TaskSummarize, TierCheapCloud, spendUnpricedModel, 0, 0, false)
	// The cache hit is free; the other call is on the $2 rate, 50,000 µUSD.
	ledger.at(julyDay(15, 8)).rate("2")
	ledger.at(julyDay(20, 9)).call(TaskCaptureClassify, TierCheapCloud, spendPricedModel, 40_000, 10_000, true)
	ledger.at(julyDay(20, 10)).call(TaskCaptureClassify, TierCheapCloud, spendPricedModel, 20_000, 5_000, false)

	now := julyDay(20, 15)
	h := ledger.at(now).handlers()
	reader := diagnosticsReader(ledger.ws)
	reportSum := reportLineSum(reader, t, h)
	summary := readSummary(reader, t, h)

	if reportSum == nil || *reportSum != 7 {
		t.Fatalf("the report's priced lines sum to %s, want 7 cents", minorText(reportSum))
	}
	if summary.CostEstMinor == nil || *summary.CostEstMinor != *reportSum {
		t.Fatalf("summary cost_est_minor = %s, want the report's %d", minorText(summary.CostEstMinor), *reportSum)
	}
	if summary.Currency != usageCurrency {
		t.Errorf("summary currency = %q, want %q", summary.Currency, usageCurrency)
	}
	if !summary.ComputedAt.Equal(now) {
		t.Errorf("summary computed_at = %s, want the meter clock %s", summary.ComputedAt, now)
	}
}

func TestMonthSpendWithNoPricedLineCarriesNoFigure(t *testing.T) {
	ledger := newSpendLedger(t, setupRateStore(t))
	ledger.at(julyDay(3, 9)).call(TaskEnrich, TierCheapCloud, spendUnpricedModel, 3_000, 1_000, false)

	h := ledger.at(julyDay(20, 15)).handlers()
	reader := diagnosticsReader(ledger.ws)
	if sum := reportLineSum(reader, t, h); sum != nil {
		t.Fatalf("the report prices %d cents for unpriced calls, want no priced line", *sum)
	}
	if summary := readSummary(reader, t, h); summary.CostEstMinor != nil {
		t.Fatalf("summary cost_est_minor = %d, want it omitted: no line carries a price", *summary.CostEstMinor)
	}
}

func TestMonthSpendRefusesAReaderWithoutTheDiagnosticsGrant(t *testing.T) {
	ledger := newSpendLedger(t, setupRateStore(t))
	ledger.at(julyDay(20, 15))
	seat := principal.WithActor(principal.WithWorkspaceID(context.Background(), ledger.ws), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:spend-test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects:  map[string]principal.ObjectGrant{"automation": {Read: true, Update: true}},
		},
	})
	if _, err := ledger.meter.MonthSpend(seat, ledger.rates); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("MonthSpend without ai_diagnostics:read = %v, want ErrPermissionDenied", err)
	}
	rec := httptest.NewRecorder()
	ledger.handlers().GetAiUsageSummary(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(seat))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("GET /ai/usage/summary without ai_diagnostics:read → %d, want 403", rec.Code)
	}
}
