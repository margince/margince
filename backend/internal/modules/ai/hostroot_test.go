// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestHostRootDropsOnlyTheVersionTheAdapterAdds(t *testing.T) {
	for _, tc := range []struct {
		name, provider, host, want string
	}{
		{"openai_compatible with /v1", providerOpenAICompatible, "https://x.example/openai/eu/v1", "https://x.example/openai/eu"},
		{"trailing slash after /v1", providerOpenAICompatible, "https://x.example/openai/eu/v1/", "https://x.example/openai/eu"},
		{"upper-case segment", providerOpenAICompatible, "https://x.example/V1", "https://x.example"},
		{"anthropic with /v1", providerAnthropic, "https://x.example/anthropic/eu/v1", "https://x.example/anthropic/eu"},
		{"openai with /v1", providerOpenAI, "https://api.openai.com/v1", "https://api.openai.com"},
		{"vllm with /v1", providerVLLM, "http://10.0.0.5:8000/v1", "http://10.0.0.5:8000"},
		{"root already", providerOpenAICompatible, "https://openrouter.ai/api", "https://openrouter.ai/api"},
		{"root with trailing slash kept as written", providerOpenAICompatible, "https://openrouter.ai/api/", "https://openrouter.ai/api/"},
		{"a host named v1 is not a path", providerOpenAICompatible, "https://v1", "https://v1"},
		{"empty", providerOpenAICompatible, "", ""},
		{"gemini keeps its version-relative base", providerGemini, "https://x.example/google/eu/v1beta", "https://x.example/google/eu/v1beta"},
		{"gemini at a /v1 base is its own", providerGemini, "https://x.example/v1", "https://x.example/v1"},
		{"jev endpoint is posted to as written", providerJevCompatible, "https://x.example/v1", "https://x.example/v1"},
		{"jev full endpoint", providerJev, "https://api.typesafe.ai/v1/systemone", "https://api.typesafe.ai/v1/systemone"},
		{"ollama paths are under /api", providerOllama, "http://localhost:11434/v1", "http://localhost:11434/v1"},
		{"unknown provider", "nope", "https://x.example/v1", "https://x.example/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostRoot(tc.provider, tc.host); got != tc.want {
				t.Errorf("hostRoot(%q, %q) = %q, want %q", tc.provider, tc.host, got, tc.want)
			}
		})
	}
}

// The registry's flag is what hostRoot reads, so it must agree with the path
// each adapter actually requests.
func TestPathsUnderV1MatchesWhatEachAdapterRequests(t *testing.T) {
	keys := map[string]string{}
	for _, env := range cloudKeyEnv {
		keys[env] = "k"
	}
	for _, d := range providerRegistry {
		// The fake opens no socket and gemini_vertex takes no host, so neither
		// has a path to compare.
		if !d.caps.has(capChat) || d.name == ProviderFake || d.name == providerGeminiVertex {
			continue
		}
		t.Run(d.name, func(t *testing.T) {
			path := firstListPath(t, ProviderConfig{Provider: d.name}, config.Static(keys))
			if got := strings.HasPrefix(path, "/v1/"); got != d.pathsUnderV1 {
				t.Errorf("%s requested %q; pathsUnderV1 is %v", d.name, path, d.pathsUnderV1)
			}
		})
	}
}

// A host stored before the write path rooted it heals on read: the model list
// asks /v1/models under the root, not /v1/v1/models.
func TestAStoredV1HostIsDialledAtItsRoot(t *testing.T) {
	stored := RoutingConfig{Providers: map[string]ProviderSettings{
		providerOpenAICompatible: {BaseURL: "/openai/eu/v1/"},
	}}
	keys := cloudKeyFor(providerOpenAICompatible, "k")
	path := firstListPath(t, providerConfigFor(stored, providerOpenAICompatible, ""), keys)
	if path != "/openai/eu/v1/models" {
		t.Errorf("model list asked %q, want /openai/eu/v1/models", path)
	}
}

func TestAProviderHostIsStoredAtItsRoot(t *testing.T) {
	const pasted, root = "https://api.langdock.com/openai/eu/v1", "https://api.langdock.com/openai/eu"
	stored, _, err := RoutingConfig{}.withProviderSettings(providerOpenAICompatible, ProviderSettings{BaseURL: pasted})
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Providers[providerOpenAICompatible].BaseURL; got != root {
		t.Errorf("stored host = %q, want %q", got, root)
	}
	healed := RoutingConfig{Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: pasted}}}
	if healed.Revision() != stored.Revision() {
		t.Error("a stored /v1 host and its root read as two revisions; a save from the read would conflict")
	}
}

// firstListPath asks cfg's adapter for its model list against a local server
// and returns the path the first request named. A relative BaseURL is taken
// as a path under that server.
func firstListPath(t *testing.T, cfg ProviderConfig, keys config.Lookup) string {
	t.Helper()
	var (
		mu    sync.Mutex
		paths []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"data":[],"models":[],"has_more":false}`))
		if err != nil {
			t.Errorf("write list body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL + cfg.BaseURL
	client, err := selectLocalBrain(cfg, keys)
	if err != nil {
		t.Fatal(err)
	}
	lister, ok := client.(model.Lister)
	if !ok {
		t.Fatalf("%s lists no models", cfg.Provider)
	}
	if _, err := lister.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) == 0 {
		t.Fatalf("%s sent no request", cfg.Provider)
	}
	return paths[0]
}
