// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// embedBodyFrom serves one embeddings call and hands back the body it was sent.
func embedBodyFrom(t *testing.T, binding ProviderConfig) map[string]json.RawMessage {
	t.Helper()
	var sent map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decoding the embeddings request: %v", err)
		}
		if _, err := w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`)); err != nil {
			t.Errorf("writing the embeddings response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	binding.BaseURL = srv.URL
	client, err := SelectBrain(binding, allCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Embed(context.Background(), model.EmbedRequest{Inputs: []string{"a document"}}); err != nil {
		t.Fatal(err)
	}
	return sent
}

// The EU preset's embeddings pin reaches the wire: parsed from the shipped file,
// built by the same selector a deployment uses, and read off the request body.
// A pin the parser accepted and the adapter dropped would read as residency in
// the file while the broker chose the region itself.
func TestTheEUPresetsEmbeddingsPinReachesTheBroker(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "config", "presets", "openrouter_cloud_eu.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := ParsePreset(raw)
	if err != nil {
		t.Fatalf("the EU preset does not parse: %v", err)
	}
	sent := embedBodyFrom(t, cfg.Embeddings.ProviderConfig)
	var provider openAICompatProviderWire
	if err := json.Unmarshal(sent["provider"], &provider); err != nil {
		t.Fatalf("the embeddings request carried no provider object (%s): %v", sent["provider"], err)
	}
	if !slices.Equal(provider.Only, []string{"mistral/eu"}) {
		t.Errorf("provider.only = %q, want the EU endpoint the preset pins", provider.Only)
	}
}

// A binding that declared nothing sends no provider object, which is the
// broker's own choice of host — the wire every other embeddings lane had before.
func TestAnUnpinnedEmbeddingsLaneSendsNoProviderObject(t *testing.T) {
	sent := embedBodyFrom(t, ProviderConfig{Provider: providerOpenAICompatible, Model: "e"})
	if _, carried := sent["provider"]; carried {
		t.Errorf("an unpinned embeddings call carried a provider object: %s", sent["provider"])
	}
}

// Only host selection is legal on the embeddings lane, and only on the broker.
func TestTheEmbeddingsLaneTakesOnlyHostSelection(t *testing.T) {
	const broker = "provider: openai_compatible, model: e, base_url: 'https://openrouter.ai/api'"
	for routing, legal := range map[string]bool{
		"{only: [mistral/eu]}":                  true,
		"{ignore: [x], allow_fallbacks: false}": true,
		"{}":                                    true,
		"{sort: throughput}":                    false,
		"{quantizations: [bf16]}":               false,
		"{require_parameters: true}":            false,
		"{preferred_max_latency_p90: 4}":        false,
		"{reasoning_effort: low}":               false,
	} {
		yaml := "profile: eu_hosted\ntiers:\n  premium: {" + broker + "}\nembeddings: {" + broker + ", routing: " + routing + "}\n"
		if _, err := ParseRouting([]byte(yaml)); (err == nil) != legal {
			t.Errorf("embeddings routing %s: accepted=%v, want %v (err: %v)", routing, err == nil, legal, err)
		}
	}
	native := "profile: eu_hosted\ntiers:\n  premium: {" + broker + "}\nembeddings: {provider: gemini, model: e, routing: {only: [x]}}\n"
	if _, err := ParseRouting([]byte(native)); err == nil {
		t.Error("a host pin on a native embeddings vendor was accepted; it fronts one host and would be sent a field it never asked for")
	}
}
