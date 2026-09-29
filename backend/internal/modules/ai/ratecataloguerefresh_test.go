// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const openRouterBase = "https://openrouter.ai/api/v1"

func openRouterBinding(model string) ProviderConfig {
	return ProviderConfig{Provider: providerOpenAICompatible, Model: model, BaseURL: openRouterBase}
}

func targetsOf(cfg RoutingConfig, sheet ...ModelRateRow) map[string]Lane {
	out := map[string]Lane{}
	for _, t := range catalogueTargets(cfg, sheet) {
		out[t.provider+"/"+t.modelID] = t.lane
	}
	return out
}

// The scope is what the installation calls through OpenRouter, lane and all. A
// model bound at another host is not priced by OpenRouter's list, whatever its id.
func TestCatalogueTargetsAreTheOpenRouterModelsTheRoutingBinds(t *testing.T) {
	cfg := RoutingConfig{
		Tiers: map[Tier]ProviderConfig{
			"premium":     openRouterBinding("anthropic/claude-opus-4.8"),
			"cheap_cloud": {Provider: providerOpenAICompatible, Model: "self/hosted", BaseURL: "https://llm.internal.test/v1"},
			"local_small": {Provider: providerOllama, Model: "gemma4"},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: openRouterBinding("openai/text-embedding-3-small")},
		Decisions:  &DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-router", BaseURL: "https://openrouter.ai/api/alpha/decisions"},
	}

	got := targetsOf(cfg)

	want := map[string]Lane{
		"openai_compatible/anthropic/claude-opus-4.8":     LaneChat,
		"openai_compatible/openai/text-embedding-3-small": LaneEmbeddings,
		"jev_compatible/typesafe/jev-router":              LaneDecisions,
	}
	if len(got) != len(want) {
		t.Fatalf("targets = %v, want exactly %v", got, want)
	}
	for key, lane := range want {
		if got[key] != lane {
			t.Errorf("target %s lane = %q, want %q", key, got[key], lane)
		}
	}
}

func TestCatalogueTargetsAddTheSheetsOpenAICompatibleRowsWithoutRefilingThem(t *testing.T) {
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{"premium": openRouterBinding("a/bound")}}
	sheet := []ModelRateRow{
		{Provider: providerOpenAICompatible, ModelID: "a/bound"},
		{Provider: providerOpenAICompatible, ModelID: "b/sheet-only"},
		{Provider: providerGemini, ModelID: "gemini-3.5-flash"},
	}

	got := targetsOf(cfg, sheet...)

	if len(got) != 2 {
		t.Fatalf("targets = %v, want the bound model and the sheet-only openai_compatible row", got)
	}
	// A bound model keeps the lane its binding names; a sheet-only row names
	// none, so the write inherits the lane the sheet already files it under.
	if got["openai_compatible/a/bound"] != LaneChat {
		t.Errorf("bound lane = %q, want chat", got["openai_compatible/a/bound"])
	}
	if lane, ok := got["openai_compatible/b/sheet-only"]; !ok || lane != "" {
		t.Errorf("sheet-only row = %q (present %t), want an empty lane", lane, ok)
	}
}

func TestCatalogueTargetsAreEmptyWhereNothingIsBoundAtOpenRouter(t *testing.T) {
	if got := targetsOf(RoutingConfig{}); len(got) != 0 {
		t.Errorf("an unbound installation has targets: %v", got)
	}
}

func pricedEntry(input, output string, cacheRead, cacheWrite *string) AvailableModel {
	return AvailableModel{InputPerMtok: &input, OutputPerMtok: &output, CacheReadPerMtok: cacheRead, CacheWritePerMtok: cacheWrite}
}

func TestCataloguePriceFilesAnUnpublishedCacheBucketAsZero(t *testing.T) {
	read := "0.5"
	in, ok := cataloguePrice(pricedEntry("5", "25", &read, nil), catalogueTarget{provider: "p", modelID: "m", lane: LaneChat})
	if !ok {
		t.Fatal("a fully stated price was refused")
	}
	if in.CacheReadUsd != "0.5" || in.CacheWriteUsd != "0" {
		t.Errorf("cache buckets = %q / %q, want 0.5 and 0 for the bucket the vendor omits", in.CacheReadUsd, in.CacheWriteUsd)
	}
	if in.Provider != "p" || in.ModelID != "m" || in.Lane != LaneChat {
		t.Errorf("identity not carried: %+v", in)
	}
}

func TestCataloguePriceRefusesWhatTheVendorDoesNotState(t *testing.T) {
	if _, ok := cataloguePrice(AvailableModel{}, catalogueTarget{}); ok {
		t.Error("an entry with no price was written as free")
	}
	input := "5"
	if _, ok := cataloguePrice(AvailableModel{InputPerMtok: &input}, catalogueTarget{}); ok {
		t.Error("an entry with no output price was written")
	}
	tooBig := "9999999999999"
	if _, ok := cataloguePrice(pricedEntry(tooBig, "1", nil, nil), catalogueTarget{}); ok {
		t.Error("a price beyond the sheet's range was written")
	}
}

// OpenRouter states a variable price as -1, which the parse refuses; the model
// must come out unpriced rather than free.
func TestAVariablePricedModelIsNotPricedFromTheBrokersList(t *testing.T) {
	body := `{"data":[{"id":"typesafe/jev-router","pricing":{"prompt":"-1","completion":"-1"}}]}`
	parsed, err := parseOpenRouterCatalogue([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	entry := toFullModel(parsed[0])
	if _, ok := cataloguePrice(entry, catalogueTarget{}); ok {
		t.Errorf("a model priced -1 by the broker was treated as priced: %+v", entry)
	}
}

func TestTheFullCatalogueViewCarriesCachePrices(t *testing.T) {
	body := `{"data":[{"id":"a/b","pricing":{"prompt":"0.000005","completion":"0.000025","input_cache_read":"0.0000005","input_cache_write":"0.00000625"}}]}`
	parsed, err := parseOpenRouterCatalogue([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := toFullModel(parsed[0])
	if got.CacheReadPerMtok == nil || *got.CacheReadPerMtok != "0.5" {
		t.Errorf("cache read = %v, want 0.5", got.CacheReadPerMtok)
	}
	if got.CacheWritePerMtok == nil || *got.CacheWritePerMtok != "6.25" {
		t.Errorf("cache write = %v, want 6.25", got.CacheWritePerMtok)
	}
}

func TestSameMicroUSDIgnoresSpellingBelowTheSheetsPrecision(t *testing.T) {
	a := SetModelRateInput{InputUsd: "5", OutputUsd: "25", CacheReadUsd: "0", CacheWriteUsd: "0"}
	b := SetModelRateInput{InputUsd: "5.000000", OutputUsd: "25.0", CacheReadUsd: "0.0", CacheWriteUsd: "0"}
	if !sameMicroUSD(a, b) {
		t.Error("the same price in another spelling read as a change")
	}
	b.OutputUsd = "25.000001"
	if sameMicroUSD(a, b) {
		t.Error("a one micro-dollar move read as unchanged")
	}
}

func outcomesOf(report RateRefreshReport) map[string]RefreshOutcome {
	out := map[string]RefreshOutcome{}
	for _, p := range report.Providers {
		out[p.Provider] = p.Outcome
	}
	return out
}

// Every provider this build knows gets a line, so an operator never has to ask
// why gemini is missing from the report.
func TestTheReportNamesEveryKnownProvider(t *testing.T) {
	report := reportProviders(map[string]*ProviderRefresh{
		providerOpenAICompatible: {Provider: providerOpenAICompatible, Updated: 2},
	}, false)

	got := outcomesOf(report)
	for _, name := range append(KnownProviders(), DecisionProviders()...) {
		if _, ok := got[name]; !ok {
			t.Errorf("provider %q has no line in the report", name)
		}
	}
	if got[providerOpenAICompatible] != RefreshUpdated {
		t.Errorf("openai_compatible = %q, want updated", got[providerOpenAICompatible])
	}
	if got[providerJevCompatible] != RefreshNotBound {
		t.Errorf("jev_compatible with nothing bound = %q, want not_bound", got[providerJevCompatible])
	}
	for _, vendor := range []string{providerGemini, providerAnthropic, providerOpenAI, providerOllama, providerVLLM, providerJev} {
		if got[vendor] != RefreshNotAvailable {
			t.Errorf("%s = %q, want not_available: it publishes no price list", vendor, got[vendor])
		}
	}
}

func TestTheReportSaysWhyABrokerProviderWroteNothing(t *testing.T) {
	cases := []struct {
		name string
		line ProviderRefresh
		down bool
		want RefreshOutcome
	}{
		{"every price already current", ProviderRefresh{Unchanged: 3}, false, RefreshUnchanged},
		{"bound but the list states no price", ProviderRefresh{}, false, RefreshNotAvailable},
		{"the list could not be read", ProviderRefresh{Updated: 1, Models: []string{"a/b"}}, true, RefreshUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.line.Provider = providerJevCompatible
			report := reportProviders(map[string]*ProviderRefresh{providerJevCompatible: &tc.line}, tc.down)
			var got ProviderRefresh
			for _, p := range report.Providers {
				if p.Provider == providerJevCompatible {
					got = p
				}
			}
			if got.Outcome != tc.want {
				t.Fatalf("outcome = %q, want %q", got.Outcome, tc.want)
			}
			if tc.down && (got.Updated != 0 || len(got.Models) != 0) {
				t.Errorf("an unreachable line claims work it did not do: %+v", got)
			}
			if got.Models == nil {
				t.Error("models is nil; the wire promises a list")
			}
		})
	}
}

func principalCtx(kind principal.PrincipalType) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: kind, ID: string(kind) + ":test"})
}

func refreshRequest(ctx context.Context) (*httptest.ResponseRecorder, *http.Request) {
	req := httptest.NewRequest(http.MethodPost, "/v1/ai-model-rates/refresh", nil).WithContext(ctx)
	return httptest.NewRecorder(), req
}

// The route is human-only three ways; this is the handler's own half. An agent
// is refused before the handler learns whether it is wired.
func TestRefreshAiModelRatesRefusesAnAgentPrincipal(t *testing.T) {
	w, r := refreshRequest(principalCtx(principal.PrincipalAgent))
	Handlers{}.RefreshAiModelRates(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("agent → %d, want 403", w.Code)
	}
}

func TestRefreshAiModelRatesAnswers501WhereNothingIsWired(t *testing.T) {
	w, r := refreshRequest(principalCtx(principal.PrincipalHuman))
	Handlers{}.RefreshAiModelRates(w, r)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("unwired → %d, want 501", w.Code)
	}
}

func TestTheContractReportKeepsProviderOrderAndModels(t *testing.T) {
	report := RateRefreshReport{Providers: []ProviderRefresh{
		{Provider: "b", Outcome: RefreshUpdated, Updated: 1, Models: []string{"x"}},
		{Provider: "a", Outcome: RefreshNotBound, Models: []string{}},
	}}
	wire := toContractRefreshReport(report)
	names := []string{wire.Providers[0].Provider, wire.Providers[1].Provider}
	if !slices.Equal(names, []string{"b", "a"}) {
		t.Errorf("providers reordered: %v", names)
	}
	if wire.Providers[0].Outcome != "updated" || wire.Providers[0].Updated != 1 || wire.Providers[0].Models[0] != "x" {
		t.Errorf("first line lost data: %+v", wire.Providers[0])
	}
}
