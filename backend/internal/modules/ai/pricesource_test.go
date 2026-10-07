// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"slices"
	"testing"
)

// A provider added without a price source would be silently unpriced forever,
// so every registry row must say: models.dev under a key the file has, the
// broker's list, or no published price at all.
func TestEveryProviderDeclaresWhereItsPricesComeFrom(t *testing.T) {
	prices := fixturePrices(t)
	for _, d := range providerRegistry {
		switch d.priceSource.kind {
		case priceSourceUnset:
			t.Errorf("%s declares no price source", d.name)
		case priceFromModelsDev:
			if _, ok := prices.byKey[d.priceSource.catalogueKey]; !ok {
				t.Errorf("%s reads models.dev key %q, which the catalogue does not have", d.name, d.priceSource.catalogueKey)
			}
		case priceFromBroker, priceNotPublished:
			if d.priceSource.catalogueKey != "" {
				t.Errorf("%s names catalogue key %q but is not priced from models.dev", d.name, d.priceSource.catalogueKey)
			}
		default:
			t.Errorf("%s declares unknown price source kind %d", d.name, d.priceSource.kind)
		}
	}
}

func TestTheVendorAPIsArePricedFromModelsDevAndTheWireBrokersFromOpenRouter(t *testing.T) {
	want := map[string]string{
		providerGemini: "google", providerGeminiVertex: "google-vertex",
		providerOpenAI: "openai", providerAnthropic: "anthropic",
	}
	for provider, key := range want {
		if got, ok := modelsDevKeyFor(provider); !ok || got != key {
			t.Errorf("modelsDevKeyFor(%s) = %q, %v; want %q", provider, got, ok, key)
		}
	}
	if got := modelsDevProviders(); len(got) != len(want) {
		t.Errorf("models.dev providers = %v, want exactly the four vendor APIs", got)
	}
	for _, p := range []string{providerOpenAICompatible, providerJevCompatible} {
		if !brokerPriced(p) {
			t.Errorf("%s is not broker-priced", p)
		}
	}
	for _, p := range []string{providerOllama, providerVLLM, providerJev, ProviderFake} {
		if brokerPriced(p) || slices.Contains(modelsDevProviders(), p) {
			t.Errorf("%s claims a published price", p)
		}
	}
}
