// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// Gemini on Vertex AI serves the Gemini models, so a model the Vertex sheet
// does not price is priced at Gemini's row; a Vertex row, once written, wins.

import (
	"context"
	"testing"
	"time"
)

func TestAVertexCallIsPricedAtGeminisRateUntilVertexHasOne(t *testing.T) {
	e := setupRateStore(t)
	ctx := context.Background()
	ws, wsCtx := e.seedWorkspace(ctx, t)
	store := e.storeFor(ws)
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	gemini := ModelRate{Provider: providerGemini, ModelID: "gemini-2.5-flash", InputPerMTokMicroUSD: 300_000, OutputPerMTokMicroUSD: 2_500_000, EffectiveDate: day}
	e.insertRate(ctx, t, gemini)

	inherited, err := store.RateFor(wsCtx, providerGeminiVertex, "gemini-2.5-flash", day)
	if err != nil {
		t.Fatal(err)
	}
	if inherited == nil || inherited.InputPerMTokMicroUSD != 300_000 {
		t.Fatalf("Vertex rate with no Vertex row = %+v, want Gemini's", inherited)
	}

	vertex := gemini
	vertex.Provider, vertex.InputPerMTokMicroUSD = providerGeminiVertex, 330_000
	e.insertRate(ctx, t, vertex)
	own, err := store.RateFor(wsCtx, providerGeminiVertex, "gemini-2.5-flash", day.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if own == nil || own.InputPerMTokMicroUSD != 330_000 {
		t.Errorf("Vertex rate with a Vertex row = %+v, want Vertex's own", own)
	}
	if other, err := store.RateFor(wsCtx, providerOpenAI, "gemini-2.5-flash", day); err != nil || other != nil {
		t.Errorf("an unrelated provider borrowed Gemini's rate: %+v, %v", other, err)
	}
}

func TestTheCostReportPricesAVertexCallAtGeminisRate(t *testing.T) {
	e := setupRateStore(t)
	ctx := context.Background()
	ws, wsCtx := e.seedWorkspace(ctx, t)
	store := e.storeFor(ws)
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	rate := ModelRate{Provider: providerGemini, ModelID: "gemini-2.5-flash", InputPerMTokMicroUSD: 300_000, OutputPerMTokMicroUSD: 2_500_000, EffectiveDate: at.Truncate(24 * time.Hour)}
	e.insertRate(ctx, t, rate)
	call := callFixture{task: TaskSummarize, tier: TierPremium, provider: providerGeminiVertex, model: "gemini-2.5-flash", tokensIn: 1000, tokensOut: 200, occurredAt: at}
	e.insertCall(ctx, t, call)

	report, err := store.CostReport(wsCtx, at.Add(-time.Hour), at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := PriceCall(Usage{TokensIn: 1000, TokensOut: 200}, rate)
	if len(report) != 1 || report[0].CostMicroUSD != want || report[0].UnpricedCalls != 0 {
		t.Errorf("report = %+v, want one line costing %d with nothing unpriced", report, want)
	}
}
