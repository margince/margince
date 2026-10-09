// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func sheetRow(provider, id, input string, source RateSource) ModelRateRow {
	return ModelRateRow{
		Provider: provider, ModelID: id, InputUsd: input, OutputUsd: "9",
		CacheReadUsd: "0.15", CacheWriteUsd: "0", Lane: LaneChat, Source: source,
	}
}

func vendorSources(t *testing.T, provider string, listed ...model.Info) PriceSources {
	t.Helper()
	src := PriceSources{
		ModelsDev: fixturePrices(t), Usable: map[string]bool{provider: true},
		Listed: map[string][]model.Info{},
	}
	if listed != nil {
		src.Listed[provider] = listed
	}
	return src
}

func writesOf(plan pricePlan) map[string]SetModelRateInput {
	out := map[string]SetModelRateInput{}
	for _, w := range plan.writes {
		out[w.Provider+"/"+w.ModelID] = w
	}
	return out
}

func TestASyncAddsOnlyListedModelsTheCataloguePricesInTheirLane(t *testing.T) {
	src := vendorSources(
		t, providerGemini,
		model.Info{ID: "gemini-3.5-flash", Lane: model.LaneChat},
		model.Info{ID: "gemini-embedding-001", Lane: model.LaneEmbeddings},
		model.Info{ID: "gemini-2.5-flash-image", Lane: model.LaneChat},
		model.Info{ID: "gemma-4-31b-it", Lane: model.LaneChat},
		model.Info{ID: "gemini-2.5-flash-preview-tts", Lane: model.LaneChat},
		model.Info{ID: "gemini-not-in-the-catalogue", Lane: model.LaneChat},
	)
	plan := planPriceSync(src, nil)

	got := writesOf(plan)
	if len(got) != 2 {
		t.Fatalf("writes = %v, want the chat and the embedding model only", got)
	}
	chat := got["gemini/gemini-3.5-flash"]
	if chat.Lane != LaneChat || chat.InputUsd != "1.500000" || chat.OutputUsd != "9.000000" ||
		chat.CacheReadUsd != "0.150000" || chat.CacheWriteUsd != "0" || chat.Source != RateSourceCatalogue {
		t.Errorf("chat write = %+v", chat)
	}
	if got["gemini/gemini-embedding-001"].Lane != LaneEmbeddings {
		t.Errorf("embedder filed as %q", got["gemini/gemini-embedding-001"].Lane)
	}
	line := plan.lines[providerGemini]
	if line.Added != 2 || line.Updated != 0 || !slices.Equal(line.Models, []string{"gemini-3.5-flash", "gemini-embedding-001"}) {
		t.Errorf("line = %+v", line)
	}
}

func TestAVendorThatStatesNoLaneTakesTheCataloguesLane(t *testing.T) {
	src := vendorSources(t, providerOpenAI,
		model.Info{ID: "gpt-5-mini"}, model.Info{ID: "text-embedding-3-small"}, model.Info{ID: "gpt-image-1"})
	got := writesOf(planPriceSync(src, nil))
	if got["openai/gpt-5-mini"].Lane != LaneChat || got["openai/text-embedding-3-small"].Lane != LaneEmbeddings {
		t.Errorf("writes = %+v", got)
	}
	if _, wrote := got["openai/gpt-image-1"]; wrote {
		t.Error("an image model was added")
	}
}

func TestAStatedLaneTheCatalogueContradictsIsNotAdded(t *testing.T) {
	src := vendorSources(t, providerGemini, model.Info{ID: "gemini-embedding-001", Lane: model.LaneChat})
	if got := writesOf(planPriceSync(src, nil)); len(got) != 0 {
		t.Errorf("writes = %v, want none: vendor and catalogue disagree on the lane", got)
	}
}

func TestAHandSetPriceIsNeverRewritten(t *testing.T) {
	src := vendorSources(t, providerGemini, model.Info{ID: "gemini-3.5-flash", Lane: model.LaneChat})
	plan := planPriceSync(src, []ModelRateRow{sheetRow(providerGemini, "gemini-3.5-flash", "9.99", RateSourceManual)})
	if len(plan.writes) != 0 {
		t.Fatalf("writes = %+v, want none", plan.writes)
	}
	if line := plan.lines[providerGemini]; line.Kept != 1 {
		t.Errorf("line = %+v, want kept 1", line)
	}
}

func TestASeedPriceIsRepricedAndAMissingFigureKeepsItsValue(t *testing.T) {
	row := sheetRow(providerGemini, "gemini-3.5-flash", "1.4", RateSourceSeed)
	row.CacheWriteUsd = "0.5"
	plan := planPriceSync(vendorSources(t, providerGemini), []ModelRateRow{row})
	w := writesOf(plan)["gemini/gemini-3.5-flash"]
	if w.InputUsd != "1.500000" || w.CacheWriteUsd != "0.5" || w.Lane != "" {
		t.Errorf("write = %+v, want 1.5 in, 0.5 cache write kept, lane inherited", w)
	}
	if line := plan.lines[providerGemini]; line.Updated != 1 || line.Added != 0 {
		t.Errorf("line = %+v", line)
	}
}

func TestAnUnchangedPriceWritesNothing(t *testing.T) {
	plan := planPriceSync(vendorSources(t, providerGemini),
		[]ModelRateRow{sheetRow(providerGemini, "gemini-3.5-flash", "1.5", RateSourceCatalogue)})
	if len(plan.writes) != 0 || plan.lines[providerGemini].Unchanged != 1 {
		t.Errorf("plan = %+v / %+v", plan.writes, plan.lines[providerGemini])
	}
}

func TestAProviderWithNoUsableKeyIsNotTouched(t *testing.T) {
	src := vendorSources(t, providerGemini, model.Info{ID: "gemini-3.5-flash", Lane: model.LaneChat})
	src.Usable = map[string]bool{}
	plan := planPriceSync(src, []ModelRateRow{sheetRow(providerGemini, "gemini-3.5-flash", "1.4", RateSourceSeed)})
	if len(plan.writes) != 0 || plan.lines[providerGemini].Outcome != RefreshNotConfigured {
		t.Errorf("plan = %+v / %+v", plan.writes, plan.lines[providerGemini])
	}
}

func TestAFailedListingStillRepricesTheSheet(t *testing.T) {
	plan := planPriceSync(vendorSources(t, providerGemini),
		[]ModelRateRow{sheetRow(providerGemini, "gemini-3.5-flash", "1.4", RateSourceSeed)})
	if line := plan.lines[providerGemini]; line.Updated != 1 || line.Added != 0 {
		t.Errorf("line = %+v, want the sheet row re-priced and nothing added", line)
	}
}

func TestAnUnreachableCatalogueWritesNothing(t *testing.T) {
	src := vendorSources(t, providerGemini, model.Info{ID: "gemini-3.5-flash", Lane: model.LaneChat})
	src.ModelsDev = ModelsDevPrices{Unreachable: true}
	plan := planPriceSync(src, []ModelRateRow{sheetRow(providerGemini, "gemini-3.5-flash", "1.4", RateSourceSeed)})
	if len(plan.writes) != 0 || plan.lines[providerGemini].Outcome != RefreshUnreachable {
		t.Errorf("plan = %+v / %+v", plan.writes, plan.lines[providerGemini])
	}
}

func TestASheetModelTheCatalogueDoesNotNameIsUnlisted(t *testing.T) {
	plan := planPriceSync(vendorSources(t, providerGemini),
		[]ModelRateRow{sheetRow(providerGemini, "gemini-3.1-pro-preview", "2", RateSourceSeed)})
	if got := plan.lines[providerGemini].Unlisted; !slices.Equal(got, []string{"gemini-3.1-pro-preview"}) {
		t.Errorf("unlisted = %v", got)
	}
}

func TestTheBrokerKeepsAHandSetPriceToo(t *testing.T) {
	src := PriceSources{
		Routing: RoutingConfig{Tiers: map[Tier]ProviderConfig{"premium": openRouterBinding("a/bound")}},
		Broker:  AvailableModels{Models: []AvailableModel{{ID: "a/bound", InputPerMtok: new("5"), OutputPerMtok: new("25")}}},
	}
	plan := planPriceSync(src, []ModelRateRow{sheetRow(providerOpenAICompatible, "a/bound", "4", RateSourceManual)})
	if len(plan.writes) != 0 || plan.lines[providerOpenAICompatible].Kept != 1 {
		t.Errorf("plan = %+v / %+v", plan.writes, plan.lines[providerOpenAICompatible])
	}
}

func TestTheReportSaysWhatTheSyncDidForEachVendor(t *testing.T) {
	report := reportProviders(map[string]*ProviderRefresh{
		providerGemini:    {Provider: providerGemini, Added: 2},
		providerAnthropic: {Provider: providerAnthropic, Kept: 1},
		providerOpenAI:    {Provider: providerOpenAI, Outcome: RefreshNotConfigured},
	}, false)
	got := outcomesOf(report)
	want := map[string]RefreshOutcome{
		providerGemini: RefreshUpdated, providerAnthropic: RefreshUnchanged, providerOpenAI: RefreshNotConfigured,
		providerGeminiVertex: RefreshNotBound, providerOllama: RefreshNotAvailable,
	}
	for provider, outcome := range want {
		if got[provider] != outcome {
			t.Errorf("%s = %q, want %q", provider, got[provider], outcome)
		}
	}
}

// A keyed vendor with nothing on its sheet and nothing listed has nothing to
// price; "set by hand" (not_available) would be false for a vendor models.dev prices.
func TestAKeyedVendorWithNothingToPriceIsNotBound(t *testing.T) {
	plan := planPriceSync(vendorSources(t, providerGemini), nil)
	report := reportProviders(plan.lines, false)
	if got := outcomesOf(report)[providerGemini]; got != RefreshNotBound {
		t.Errorf("gemini = %q, want not_bound", got)
	}
}

// An installation with nothing at OpenRouter and no keyed vendor reads neither
// public list: a sync with nothing to price sends nothing out.
func TestASyncWithNothingToPriceReadsNoCatalogue(t *testing.T) {
	broker, modelsDev := cataloguesToRead(RoutingConfig{}, map[string]bool{providerOllama: true})
	if broker || modelsDev {
		t.Errorf("read broker=%v models.dev=%v, want neither", broker, modelsDev)
	}
	atOpenRouter := RoutingConfig{Tiers: map[Tier]ProviderConfig{"premium": openRouterBinding("a/bound")}}
	if broker, _ := cataloguesToRead(atOpenRouter, nil); !broker {
		t.Error("a binding at OpenRouter did not read its list")
	}
	if _, modelsDev := cataloguesToRead(RoutingConfig{}, map[string]bool{providerGemini: true}); !modelsDev {
		t.Error("a keyed vendor did not read models.dev")
	}
}

// A write that yields to a price typed since the read is reported as kept, so the
// recorded run never claims a price it did not write.
func TestAWriteThatYieldedIsReportedKept(t *testing.T) {
	src := vendorSources(t, providerGemini, model.Info{ID: "gemini-3.5-flash", Lane: model.LaneChat})
	plan := planPriceSync(src, nil)
	plan.yielded(plan.writes[0])
	line := plan.lines[providerGemini]
	if line.Added != 0 || line.Kept != 1 || len(line.Models) != 0 {
		t.Errorf("line = %+v, want the add turned into a kept price", line)
	}
}
