// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// The broker list and models.dev, read from a fake rather than the network.
type fixedPriceList []byte

func (f fixedPriceList) Fetch(context.Context) ([]byte, error) { return f, nil }

const sweepModelsDev = `{"google": {"models": {
  "gemini-3.5-flash": {"id": "gemini-3.5-flash", "family": "gemini-flash", "modalities": {"output": ["text"]}, "cost": {"input": 1.6, "output": 9, "cache_read": 0.16}}
}}}`

func newTestPriceSweep(e *integration.Env) *aiPriceSyncSweepWorker {
	clock := systemClock{}
	cats := aiPriceCatalogues{
		broker:    ai.NewModelCatalogueOver(fixedPriceList(`{"data":[{"id":"x/y","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`), clock),
		modelsDev: ai.NewModelsDevCatalogueOver(fixedPriceList(sweepModelsDev), clock),
	}
	noEnv := func(string) string { return "" }
	log := slog.New(slog.DiscardHandler)
	return &aiPriceSyncSweepWorker{pool: e.Pool, sync: newAIPriceSync(e.Pool, nil, noEnv, log, cats), log: log}
}

// keyGemini makes Gemini usable without a real key: a sealed ref exists, and
// with no vault and no environment the listing fails before any network call.
func keyGemini(t *testing.T, e *integration.Env) {
	t.Helper()
	if err := settings.Set(e.Admin(), NewSettingsStore(e.Pool), ai.ProviderKeys, map[string]string{"gemini": "ref-for-test"}); err != nil {
		t.Fatal(err)
	}
}

func seedPrice(t *testing.T, e *integration.Env, input string, source ai.RateSource) {
	t.Helper()
	if _, err := ai.NewRateStore(e.DB()).SetModelRate(e.Admin(), ai.SetModelRateInput{
		Provider: "gemini", ModelID: "gemini-3.5-flash", InputUsd: input, OutputUsd: "9",
		CacheReadUsd: "0.15", CacheWriteUsd: "0", Source: source,
	}); err != nil {
		t.Fatal(err)
	}
}

func geminiInForce(t *testing.T, e *integration.Env) ai.ModelRateRow {
	t.Helper()
	rows, err := ai.NewRateStore(e.DB()).ListEffectiveModelRates(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Provider == "gemini" && r.ModelID == "gemini-3.5-flash" {
			return r
		}
	}
	t.Fatal("no gemini-3.5-flash price in force")
	return ai.ModelRateRow{}
}

func lastRun(t *testing.T, e *integration.Env) *ai.LastPriceSync {
	t.Helper()
	run, err := settings.Get(e.Admin(), NewSettingsStore(e.Pool), ai.PriceSyncLastRun)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestTheDailySweepRepricesASeededPriceAndRecordsTheRun(t *testing.T) {
	e := integration.Setup(t)
	keyGemini(t, e)
	seedPrice(t, e, "1.5", ai.RateSourceSeed)

	if err := newTestPriceSweep(e).syncWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}

	if got := geminiInForce(t, e); got.InputUsd != "1.6" || got.Source != ai.RateSourceCatalogue {
		t.Errorf("price in force = %+v, want 1.6 from the catalogue", got)
	}
	run := lastRun(t, e)
	if run == nil || run.Trigger != ai.PriceSyncScheduled || time.Since(run.RanAt) > time.Hour {
		t.Fatalf("last run = %+v, want a scheduled run just now", run)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'ai_model_rate'
		AND actor_type = 'system' AND actor_id = 'system:ai-price-sync' AND after->>'model_id' = 'gemini-3.5-flash'`); n != 1 {
		t.Errorf("%d price audit rows under the sweep's own name, want 1", n)
	}
}

func TestTheDailySweepLeavesAnInstallationWithAutoSyncOffAlone(t *testing.T) {
	e := integration.Setup(t)
	keyGemini(t, e)
	seedPrice(t, e, "1.5", ai.RateSourceSeed)
	if err := settings.Set(e.Admin(), NewSettingsStore(e.Pool), ai.PriceSyncSettings, ai.PriceSyncConfig{AutoSync: false}); err != nil {
		t.Fatal(err)
	}

	if err := newTestPriceSweep(e).syncWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}

	if got := geminiInForce(t, e); got.InputUsd != "1.5" {
		t.Errorf("price moved to %s with auto-sync off", got.InputUsd)
	}
	if run := lastRun(t, e); run != nil {
		t.Errorf("a run was recorded with auto-sync off: %+v", run)
	}
}

func TestAHandSetPriceSurvivesTheDailySweep(t *testing.T) {
	e := integration.Setup(t)
	keyGemini(t, e)
	seedPrice(t, e, "9.99", ai.RateSourceManual)

	if err := newTestPriceSweep(e).syncWorkspace(context.Background(), e.WS); err != nil {
		t.Fatal(err)
	}

	if got := geminiInForce(t, e); got.InputUsd != "9.99" || got.Source != ai.RateSourceManual {
		t.Errorf("hand-set price = %+v, want 9.99 kept", got)
	}
	run := lastRun(t, e)
	if run == nil {
		t.Fatal("no run recorded")
	}
	found := false
	for _, line := range run.Report.Providers {
		if line.Provider == "gemini" {
			found = true
			if line.Kept != 1 {
				t.Errorf("gemini line = %+v, want kept 1", line)
			}
		}
	}
	if !found {
		t.Error("the run has no gemini line")
	}
}
