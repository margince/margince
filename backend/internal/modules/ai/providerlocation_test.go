// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"strings"
	"testing"
)

// A Vertex location is where Google processes the call, which is the
// provider's answer for every lane on it, so the lanes' copies move there.
func TestLift_VertexLocationMovesToTheProvider(t *testing.T) {
	log, buf := warnings()
	lifted := vertexRouting("europe-west4").liftLaneProviderFields(log)

	if got := lifted.Providers[providerGeminiVertex].Location; got != "europe-west4" {
		t.Errorf("provider location = %q, want europe-west4", got)
	}
	if got := lifted.Tiers[TierPremium].Location; got != "" {
		t.Errorf("tier premium kept location %q after the lift", got)
	}
	if got := lifted.Embeddings.Location; got != "" {
		t.Errorf("the embeddings lane kept location %q after the lift", got)
	}
	if lines := warnLines(buf); len(lines) != 0 {
		t.Errorf("an agreeing lift warned: %q", lines)
	}
}

func TestLift_DisagreeingTierLocationsFirstInOrderWinsAndWarns(t *testing.T) {
	cfg := vertexRouting("europe-west4")
	cfg.Tiers[TierFrontier] = ProviderConfig{Provider: providerGeminiVertex, Location: "eu", Model: "gemini-3.1-pro-preview"}
	log, buf := warnings()

	lifted := cfg.liftLaneProviderFields(log)

	if got := lifted.Providers[providerGeminiVertex].Location; got != "eu" {
		t.Errorf("provider location = %q, want eu (tier frontier sorts first)", got)
	}
	if lines := warnLines(buf); len(lines) != 1 || !strings.Contains(lines[0], "tier premium") {
		t.Errorf("warnings = %q, want one, for tier premium", lines)
	}
}

// Vertex serves an embedding model at fewer locations than a chat model, so
// the embeddings lane keeps a location of its own rather than being moved.
func TestLift_TheEmbeddingsLaneKeepsItsOwnLocation(t *testing.T) {
	cfg := vertexRouting("eu")
	cfg.Embeddings.Location = "europe-west4"
	log, buf := warnings()

	lifted := cfg.liftLaneProviderFields(log)

	if got := lifted.Providers[providerGeminiVertex].Location; got != "eu" {
		t.Errorf("provider location = %q, want the tiers' eu", got)
	}
	if got := lifted.Embeddings.Location; got != "europe-west4" {
		t.Errorf("embeddings location = %q, want its own europe-west4 kept", got)
	}
	if lines := warnLines(buf); len(lines) != 0 {
		t.Errorf("an embeddings override warned: %q", lines)
	}
	resolved, err := cfg.finalize()
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if resolved.Tiers[TierPremium].Location != "eu" || resolved.Embeddings.Location != "europe-west4" {
		t.Errorf("resolved premium %q, embeddings %q; want eu and europe-west4", resolved.Tiers[TierPremium].Location, resolved.Embeddings.Location)
	}
}

// With no tier on Vertex, a provider entry that already names a location is
// the provider's answer, and the embeddings lane's differing one stays its own.
func TestLift_AnEmbeddingsOnlyProviderKeepsTheLanesOwnLocation(t *testing.T) {
	cfg := RoutingConfig{
		Profile:    ProfileEUHosted,
		Tiers:      map[Tier]ProviderConfig{},
		Embeddings: EmbeddingsConfig{Provider: providerGeminiVertex, Model: "gemini-embedding-001", Location: "europe-west4"},
		Providers:  map[string]ProviderSettings{providerGeminiVertex: {Location: "eu"}},
	}
	log, _ := warnings()

	lifted := cfg.liftLaneProviderFields(log)

	if got := lifted.Providers[providerGeminiVertex].Location; got != "eu" {
		t.Errorf("provider location = %q, want its stored eu", got)
	}
	if got := lifted.Embeddings.Location; got != "europe-west4" {
		t.Errorf("embeddings location = %q, want its own europe-west4 kept", got)
	}
}

func TestResolve_EveryVertexLaneReadsItsProvidersLocation(t *testing.T) {
	cfg := vertexRouting("")
	cfg.Providers = map[string]ProviderSettings{providerGeminiVertex: {Location: "europe-west4"}}

	resolved, err := cfg.finalize()
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}

	for label, got := range map[string]string{
		"tier premium":      resolved.Tiers[TierPremium].Location,
		embeddingsLaneLabel: resolved.Embeddings.Location,
	} {
		if got != "europe-west4" {
			t.Errorf("%s location = %q, want the provider's", label, got)
		}
	}
	if got := resolved.Tiers[TierLocalSmall].Location; got != "" {
		t.Errorf("an ollama tier picked up location %q", got)
	}
}

func TestValidateProviderEntries_LocationIsVertexOnly(t *testing.T) {
	cfg := RoutingConfig{Providers: map[string]ProviderSettings{providerOpenAICompatible: {Location: "eu"}}}
	if err := cfg.validateProviderEntries(); err == nil || !strings.Contains(err.Error(), "location") {
		t.Errorf("a location on openai_compatible: err = %v, want a refusal naming location", err)
	}
	cfg = RoutingConfig{Providers: map[string]ProviderSettings{providerGeminiVertex: {Location: "eu"}}}
	if err := cfg.validateProviderEntries(); err != nil {
		t.Errorf("a location on gemini_vertex was refused: %v", err)
	}
}

// Moving the provider's location moves every lane on it, so each bound model
// is asked at the new location before the move is stored.
func TestChangingTheVertexLocationAsksGoogleAboutEveryBoundModel(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servesEverything(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	stored := vertexRouting("europe-west4").canonical()

	if err := store.probeProviderSettings(context.Background(), stored, providerGeminiVertex, ProviderSettings{Location: "europe-west1"}); err != nil {
		t.Fatalf("a location serving every model was refused: %v", err)
	}
	if n := google.probes(); n != 2 {
		t.Errorf("moving the location probed %d bindings, want 2 (the premium model and the embedder)", n)
	}
}

func TestSettingAnotherProvidersHostAsksGoogleNothing(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servesEverything(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}

	err := store.probeProviderSettings(context.Background(), vertexRouting("europe-west4").canonical(), providerOllama, ProviderSettings{BaseURL: "http://localhost:11434"})
	if err != nil {
		t.Fatalf("an unrelated provider edit was refused: %v", err)
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("an unrelated provider edit sent %d request(s) to Google", n)
	}
}

// An old client writes back each tier's location; one equal to the provider's
// is the provider's, a different one is changed on the provider now.
func TestReplace_OldClientTierLocation(t *testing.T) {
	t.Parallel()
	stored := vertexRouting("europe-west4").canonical()
	equal := vertexRouting("europe-west4")
	if _, served, err := equal.replacing(stored); err != nil || served.Tiers[TierPremium].Location != "europe-west4" {
		t.Errorf("an equal location: err = %v, premium served at %q", err, served.Tiers[TierPremium].Location)
	}
	moved := vertexRouting("europe-west4")
	moved.Tiers[TierPremium] = ProviderConfig{Provider: providerGeminiVertex, Location: "europe-west1", Model: "gemini-3.5-flash"}
	_, _, err := moved.replacing(stored)
	wantRefusal(t, err, CodeMovedToProvider, "tier premium", "location", "PUT /ai/provider-settings/gemini_vertex")
}
