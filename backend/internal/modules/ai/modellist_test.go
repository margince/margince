// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Each vendor's list endpoint, against its own published envelope.
//
// The five wire shapes disagree about everything — the envelope key, whether an
// id is namespaced, whether the vendor says what a model is FOR — and every one
// of those disagreements is a place a hand-written decoder is wrong in a way
// nothing else in this tree would notice: a picker that silently lists nothing
// looks exactly like a vendor with no models.

// listerFor builds one adapter against a stub serving `path`, and fails the
// test if the adapter asks for a different one — the path IS the contract with
// the vendor, and an adapter that asks the wrong URL gets a 404 that this
// package would otherwise report as an ordinary "unreachable".
func listerFor(
	t *testing.T,
	cfg ProviderConfig,
	path string,
	body map[string]any,
) model.Lister {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("asked %s, expected %s", r.URL.Path, path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encoding fixture: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	client, err := SelectBrain(cfg, cloudKeyFor(cfg.Provider, "test-key"))
	if err != nil {
		t.Fatal(err)
	}
	lister, ok := client.(model.Lister)
	if !ok {
		t.Fatalf("%s does not implement ModelLister", cfg.Provider)
	}
	return lister
}

func idsOf(models []model.Info) []string {
	out := make([]string, 0, len(models))
	for _, m := range models {
		out = append(out, m.ID)
	}
	return out
}

func TestAnthropicListsItsOwnCatalogue(t *testing.T) {
	lister := listerFor(t, ProviderConfig{Provider: providerAnthropic}, "/v1/models", map[string]any{
		"data": []map[string]string{
			{"id": "claude-opus-4-8", "display_name": "Claude Opus 4.8"},
			{"id": "claude-sonnet-4-6", "display_name": "Claude Sonnet 4.6"},
		},
		"has_more": false,
	})
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The vendor's own order is kept: this endpoint returns newest first, which
	// is the order somebody looking for "the new one" is reading for.
	if got := idsOf(models); len(got) != 2 || got[0] != "claude-opus-4-8" {
		t.Fatalf("ids wrong: %v", got)
	}
	if models[0].DisplayName != "Claude Opus 4.8" {
		t.Fatalf("display name dropped: %+v", models[0])
	}
	// Every model on the Messages API serves chat, so the lane is stated rather
	// than left unknown — an embeddings lane must not be offered one of these.
	if models[0].Lane != model.LaneChat {
		t.Fatalf("lane not stated as chat: %q", models[0].Lane)
	}
}

func TestAnthropicSendsTheVersionHeaderTheAPIRequires(t *testing.T) {
	var version, key string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version = r.Header.Get("anthropic-version")
		key = r.Header.Get("x-api-key")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{}}); err != nil {
			t.Errorf("encoding fixture: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := SelectBrain(
		ProviderConfig{Provider: providerAnthropic, BaseURL: srv.URL},
		cloudKeyFor(providerAnthropic, "sk-test"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.(model.Lister).ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Anthropic refuses a request with no version header, and the refusal would
	// reach a reader as "this vendor did not answer".
	if version != anthropicAPIVersion {
		t.Fatalf("anthropic-version header %q, want %q", version, anthropicAPIVersion)
	}
	if key != "sk-test" {
		t.Fatalf("x-api-key not sent: %q", key)
	}
}

func TestOpenAIWireListsAndCarriesNoLane(t *testing.T) {
	lister := listerFor(t, ProviderConfig{Provider: providerOpenAI}, "/v1/models", map[string]any{
		"data": []map[string]string{
			{"id": "gpt-5.4-mini"},
			{"id": "text-embedding-3-small"},
		},
	})
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(models); len(got) != 2 || got[0] != "gpt-5.4-mini" {
		t.Fatalf("ids wrong: %v", got)
	}
	// The list is undifferentiated — an embedder arrives beside a chat model
	// with nothing on the row telling them apart. Claiming a lane here would
	// offer one of them to a tier it cannot serve.
	for _, m := range models {
		if m.Lane != "" {
			t.Fatalf("lane claimed where the vendor states none: %+v", m)
		}
	}
}

func TestBrokerListKeepsItsLabelAndDropsItsPrices(t *testing.T) {
	lister := listerFor(t,
		ProviderConfig{Provider: providerOpenAICompatible},
		"/v1/models",
		map[string]any{"data": []map[string]any{{
			"id":   "anthropic/claude-opus-4.8",
			"name": "Anthropic: Claude Opus 4.8",
			// A broker publishes prices here. They are deliberately not read:
			// the effective-dated sheet is what this product costs calls
			// against, and a second price arriving by another route is two
			// answers to one question.
			"pricing": map[string]string{"prompt": "0.000005", "completion": "0.000025"},
		}}},
	)
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "anthropic/claude-opus-4.8" {
		t.Fatalf("ids wrong: %v", idsOf(models))
	}
	if models[0].DisplayName != "Anthropic: Claude Opus 4.8" {
		t.Fatalf("broker label dropped: %+v", models[0])
	}
}

func TestGeminiUnwrapsTheNamespacedIDAndReadsTheLane(t *testing.T) {
	lister := listerFor(t, ProviderConfig{Provider: providerGemini}, "/models", map[string]any{
		"models": []map[string]any{
			{
				"name":                       "models/gemini-4.0-flash",
				"displayName":                "Gemini 4.0 Flash",
				"supportedGenerationMethods": []string{"generateContent", "countTokens"},
			},
			{
				"name":                       "models/gemini-embedding-001",
				"supportedGenerationMethods": []string{"embedContent"},
			},
			// Neither: the catalog also carries token counters and tuned-model
			// bases, and calling one of those a chat model would offer a binding
			// that cannot serve a call.
			{
				"name":                       "models/aqa",
				"supportedGenerationMethods": []string{"generateAnswer"},
			},
		},
	})
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The wire namespaces every id; a binding names the bare one.
	want := []string{"gemini-4.0-flash", "gemini-embedding-001", "aqa"}
	for i, id := range want {
		if models[i].ID != id {
			t.Fatalf("id %d is %q, want %q", i, models[i].ID, id)
		}
	}
	if models[0].Lane != model.LaneChat {
		t.Fatalf("generateContent should read as chat: %q", models[0].Lane)
	}
	if models[1].Lane != model.LaneEmbeddings {
		t.Fatalf("embedContent should read as embeddings: %q", models[1].Lane)
	}
	if models[2].Lane != "" {
		t.Fatalf("a model that serves neither must claim no lane: %q", models[2].Lane)
	}
}

// The catalog runs past one page, and a reader who was shown only the first
// would be told a current model does not exist.
func TestGeminiFollowsItsPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := map[string]any{
			"models":        []map[string]any{{"name": "models/first"}},
			"nextPageToken": "more",
		}
		if r.URL.Query().Get("pageToken") == "more" {
			page = map[string]any{"models": []map[string]any{{"name": "models/second"}}}
		}
		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Errorf("encoding fixture: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := SelectBrain(
		ProviderConfig{Provider: providerGemini, BaseURL: srv.URL},
		cloudKeyFor(providerGemini, "k"),
	)
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.(model.Lister).ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := idsOf(models); len(got) != 2 || got[1] != "second" {
		t.Fatalf("pages not followed: %v", got)
	}
}

func TestOllamaListsWhatIsPulledOntoTheHost(t *testing.T) {
	lister := listerFor(t, ProviderConfig{Provider: providerOllama}, "/api/tags", map[string]any{
		"models": []map[string]any{
			{"name": "gemma3:latest", "size": 3338801718},
			{"name": "bge-m3:latest"},
		},
	})
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The TAG is the id a binding names — `gemma3` and `gemma3:latest` are
	// different strings to the server, and trimming the tag here would offer one
	// that the call then cannot resolve.
	if got := idsOf(models); len(got) != 2 || got[0] != "gemma3:latest" {
		t.Fatalf("ids wrong: %v", got)
	}
}

// A vendor that refuses is an error, and its BODY never travels with it: the
// response on this endpoint is as often a proxy's HTML as it is a sentence, and
// a redirected URL can carry the key.
func TestListModelsReportsTheStatusAndNotTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// A vendor's error body on this endpoint is as often a proxy's HTML as
		// it is a sentence, and this one carries something that must not reach
		// a log.
		if _, err := fmt.Fprint(w, `<html>invalid api key sk-live-abcdef</html>`); err != nil {
			t.Errorf("writing the fixture body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := SelectBrain(
		ProviderConfig{Provider: providerOpenAI, BaseURL: srv.URL},
		cloudKeyFor(providerOpenAI, "k"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.(model.Lister).ListModels(context.Background())
	if err == nil {
		t.Fatal("a 401 must be an error")
	}
	if got := err.Error(); !strings.Contains(got, "401") || strings.Contains(got, "sk-live") {
		t.Fatalf("error should carry the status and nothing from the body: %q", got)
	}
}

// The fake answers offline, which is what lets the surface be exercised with no
// vendor, no key and no network.
func TestFakeClientPublishesBothLanes(t *testing.T) {
	models, err := NewFakeClient().ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lanes := map[string]bool{}
	for _, m := range models {
		lanes[m.Lane] = true
	}
	if !lanes[model.LaneChat] || !lanes[model.LaneEmbeddings] {
		t.Fatalf("the fake must publish one of each lane: %+v", models)
	}
}
