// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Every per-provider fact this package answers, written out once as the
// expectation. Each table the package reads is a projection of the registry;
// the values here are what those tables must say, so a projection that drifts
// from them is a behaviour change, not a refactor.

import (
	"maps"
	"slices"
	"testing"
)

func TestTheProviderListKeepsItsOrder(t *testing.T) {
	// The order is visible: SelectBrain's refusal lists providers in it, and
	// the config schema's enum was written in it.
	want := []string{ProviderFake, providerAnthropic, providerOllama, providerVLLM, providerOpenAICompatible, providerOpenAI, providerGemini}
	if !slices.Equal(knownProviders, want) {
		t.Fatalf("knownProviders = %v, want %v", knownProviders, want)
	}
	if got, want := DecisionProviders(), []string{providerJev, providerJevCompatible}; !slices.Equal(got, want) {
		t.Fatalf("DecisionProviders() = %v, want %v", got, want)
	}
}

// A decision adapter answers no chat call, so no chat-side table may name it.
func TestADecisionProviderIsKnownOnlyToTheDecisionLane(t *testing.T) {
	for _, provider := range DecisionProviders() {
		if slices.Contains(knownProviders, provider) || localProviders[provider] {
			t.Errorf("%s is offered to a chat tier", provider)
		}
		if _, carried := wireCarriage()[provider]; carried {
			t.Errorf("%s declares a chat carriage", provider)
		}
	}
}

// Every key slot is one row on the key screen, and only a jev_compatible key
// is optional: a self-hosted Jev-wire server needs none.
func TestOnlyTheJevCompatibleKeyIsOptional(t *testing.T) {
	if got, want := CloudProvidersNeedingKeys(), []string{providerAnthropic, providerOpenAICompatible, providerOpenAI, providerGemini, providerJev, providerJevCompatible}; !slices.Equal(got, want) {
		t.Errorf("CloudProvidersNeedingKeys() = %v, want %v", got, want)
	}
	for _, provider := range CloudProvidersNeedingKeys() {
		if got, want := keyIsOptional(provider), provider == providerJevCompatible; got != want {
			t.Errorf("keyIsOptional(%s) = %v, want %v", provider, got, want)
		}
	}
}

func TestEachProviderFactIsWhatTheTablesSaid(t *testing.T) {
	assertMap(t, "localProviders", localProviders, map[string]bool{
		ProviderFake: true, providerOllama: true, providerVLLM: true,
	})
	assertMap(t, "providerEgress", providerEgress, map[string]egressClass{
		ProviderFake: egressPublicOnly, providerAnthropic: egressPublicOnly,
		providerOpenAI: egressPublicOnly, providerGemini: egressPublicOnly,
		providerOllama: egressOperatorEndpoint, providerVLLM: egressOperatorEndpoint,
		providerOpenAICompatible: egressOperatorEndpoint,
		providerJev:              egressPublicOnly, providerJevCompatible: egressOperatorEndpoint,
	})
	assertMap(t, "cloudKeyEnv", cloudKeyEnv, map[string]string{
		providerAnthropic: "ANTHROPIC_API_KEY", providerOpenAI: "OPENAI_API_KEY",
		providerGemini: "GEMINI_API_KEY", providerOpenAICompatible: "OPENAI_COMPATIBLE_API_KEY",
		providerJev: "TYPESAFE_API_KEY", providerJevCompatible: "JEV_COMPATIBLE_API_KEY",
	})
	assertMap(t, "servedSource", servedSource, map[string]string{
		providerAnthropic: servedIdentitySourceResponse, providerOllama: servedIdentitySourceResponse,
		providerGemini: servedIdentitySourceResponse, providerOpenAI: servedIdentitySourceResponse,
		providerOpenAICompatible: servedIdentitySourceEcho, providerVLLM: servedIdentitySourceEcho,
		ProviderFake: servedIdentitySourceResponse, providerJev: servedIdentitySourceResponse,
		providerJevCompatible: servedIdentitySourceEcho,
	})
	assertMap(t, "localBaseURLDefaults", localBaseURLDefaults, map[string]string{
		providerOllama: defaultOllamaBaseURL, providerVLLM: defaultVLLMBaseURL,
	})
	if got, want := slices.Sorted(maps.Keys(wildcardWires)), []string{ProviderFake, providerOllama, providerOpenAICompatible, providerVLLM}; !slices.Equal(got, want) {
		t.Errorf("wildcardWires keys = %v, want %v", got, want)
	}
	carriage := wireCarriage()
	wantCarriage := map[string][]string{
		ProviderFake: carriesImagesAndPDF, providerAnthropic: anthropicCarries,
		providerOllama: carriesImages, providerVLLM: carriesImages,
		providerOpenAICompatible: carriesImages, providerOpenAI: openAICarries,
		providerGemini: geminiCarries,
	}
	if len(carriage) != len(wantCarriage) {
		t.Errorf("wireCarriage has %d rows, want %d", len(carriage), len(wantCarriage))
	}
	for provider, want := range wantCarriage {
		if !slices.Equal(carriage[provider], want) {
			t.Errorf("wireCarriage[%s] = %v, want %v", provider, carriage[provider], want)
		}
	}
}

func TestVendorHostedAndDefaultModelAnswerPerProvider(t *testing.T) {
	vendorHosted := map[string]bool{providerAnthropic: true, providerOpenAI: true, providerGemini: true}
	defaultModel := map[string]string{providerOllama: defaultOllamaModel, providerVLLM: defaultVLLMModel}
	for _, provider := range knownProviders {
		if got := providerIsVendorHosted(provider); got != vendorHosted[provider] {
			t.Errorf("providerIsVendorHosted(%q) = %v, want %v", provider, got, vendorHosted[provider])
		}
		if got := providerDefaultModel(provider); got != defaultModel[provider] {
			t.Errorf("providerDefaultModel(%q) = %q, want %q", provider, got, defaultModel[provider])
		}
	}
	if providerIsVendorHosted("no-such-provider") || providerDefaultModel("no-such-provider") != "" {
		t.Error("an unknown provider must answer the zero value, never borrow a neighbour's")
	}
}

func TestOnlyTheFakeIsHiddenFromThePublicProfile(t *testing.T) {
	for _, provider := range knownProviders {
		_, public := publicProvider(provider)
		if want := provider != ProviderFake; public != want {
			t.Errorf("publicProvider(%q) public = %v, want %v", provider, public, want)
		}
	}
	if _, public := publicProvider("no-such-provider"); public {
		t.Error("an unknown provider must not be named by the public profile")
	}
}

func assertMap[V comparable](t *testing.T, name string, got, want map[string]V) {
	t.Helper()
	if !maps.Equal(got, want) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}
