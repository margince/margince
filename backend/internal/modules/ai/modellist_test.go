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

// A name this surface cannot ask AT ALL — an adapter that does not exist — is
// `not_published` rather than `unreachable`.
//
// The difference is the whole point of the closed vocabulary: `unreachable`
// says the vendor was asked and did not answer, and a reader who saw that for a
// typo would go looking for a network fault behind a provider nothing called.
func TestAnUnknownProviderIsNotReportedAsUnreachable(t *testing.T) {
	if got := unavailableFor(errNoProviderKey); got != AvailabilityNoKey {
		t.Fatalf("a missing credential is %q, want no_key", got)
	}
	if got := unavailableFor(errNoBaseURL); got != AvailabilityNoEndpoint {
		t.Fatalf("a missing host is %q, want no_endpoint", got)
	}
	_, err := SelectBrain(ProviderConfig{Provider: "not-a-vendor"}, noCloudKeys())
	if err == nil {
		t.Fatal("SelectBrain accepted a provider that does not exist")
	}
	if got := unavailableFor(err); got != AvailabilityNotPublished {
		t.Fatalf("an unknown adapter is %q, want not_published", got)
	}
}

// Two lanes may name one vendor at two hosts, and Go randomises map iteration —
// so the host this read picks has to be the same one every time, or the picker's
// list changes under a reader who touched nothing.
func TestTheHostAskedIsStableAcrossReads(t *testing.T) {
	cfg := RoutingConfig{
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerOpenAICompatible, BaseURL: "https://b.example"},
			TierCheapCloud: {Provider: providerOpenAICompatible, BaseURL: "https://a.example"},
			TierFrontier:   {Provider: providerOpenAICompatible, BaseURL: "https://c.example"},
		},
	}
	first := boundProviderConfig(cfg, providerOpenAICompatible, "").BaseURL
	if first == "" {
		t.Fatal("a bound vendor answered with no host")
	}
	// Repeated rather than asserted against one literal: WHICH host wins is
	// arbitrary and may change; that the same one wins every time is the
	// property.
	for range 32 {
		if got := boundProviderConfig(cfg, providerOpenAICompatible, "").BaseURL; got != first {
			t.Fatalf("the host moved between reads: %q then %q", first, got)
		}
	}

	// And naming the lane is exact rather than stable-but-arbitrary: the picker
	// opened on `premium` is answered for the host `premium` points at, which is
	// the whole reason the lane travels with the request.
	for tier, want := range map[Tier]string{
		TierPremium:    "https://b.example",
		TierCheapCloud: "https://a.example",
		TierFrontier:   "https://c.example",
	} {
		got := boundProviderConfig(cfg, providerOpenAICompatible, string(tier)).BaseURL
		if got != want {
			t.Fatalf("lane %s was asked at %q, want its own %q", tier, got, want)
		}
	}
}

// A lane that names the vendor with NO host of its own means the adapter's
// default, and that is an answer — not an absent binding to look past.
//
// Looking past it reached for a sibling lane's override and asked the wrong
// host, which is the defect naming the lane exists to prevent, arrived at from
// the other side.
func TestALaneWithNoHostMeansTheAdapterDefaultRatherThanASibling(t *testing.T) {
	cfg := RoutingConfig{
		Tiers: map[Tier]ProviderConfig{
			// Reached wherever the adapter reaches this vendor by default.
			TierPremium: {Provider: providerGemini},
			// A sibling on the same vendor that DOES override the host.
			TierFrontier: {Provider: providerGemini, BaseURL: "https://proxy.example"},
		},
	}
	if got := boundProviderConfig(cfg, providerGemini, string(TierPremium)).BaseURL; got != "" {
		t.Fatalf("premium was asked at %q; its own binding names no host, so the answer is the adapter's default", got)
	}
	// The sibling still gets its own, so this is not the guard being too wide.
	if got := boundProviderConfig(cfg, providerGemini, string(TierFrontier)).BaseURL; got != "https://proxy.example" {
		t.Fatalf("frontier was asked at %q, want its own override", got)
	}
	// And a lane naming a DIFFERENT vendor falls through, because the question
	// is about the vendor rather than about that lane.
	cfg.Tiers[TierCheapCloud] = ProviderConfig{Provider: providerAnthropic}
	if got := boundProviderConfig(cfg, providerGemini, string(TierCheapCloud)).BaseURL; got != "https://proxy.example" {
		t.Fatalf("a lane on another vendor should fall through to this vendor's own binding, got %q", got)
	}
}

// A vendor this installation has not bound falls back to the adapter's own
// default, which is what makes the picker useful before anything is bound.
func TestAnUnboundVendorFallsBackToTheAdapterDefault(t *testing.T) {
	cfg := RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierPremium: {Provider: providerGemini, Model: "gemini-3.5-flash"},
	}}
	if got := boundProviderConfig(cfg, providerAnthropic, ""); got.BaseURL != "" {
		t.Fatalf("an unbound vendor invented a host: %q", got.BaseURL)
	}
	// And the embeddings lane's host is found too — it binds separately, and a
	// broker reached only there would otherwise be asked at the wrong address.
	cfg.Embeddings = EmbeddingsConfig{ProviderConfig: ProviderConfig{
		Provider: providerOpenAICompatible, BaseURL: "https://embed.example",
	}}
	if got := boundProviderConfig(cfg, providerOpenAICompatible, ""); got.BaseURL != "https://embed.example" {
		t.Fatalf("the embeddings host was not found: %q", got.BaseURL)
	}
}

// The cap is the picker's bound, and it binds every vendor — including the one
// whose list arrives in a single unpaginated body.
func TestOllamaObeysTheListCap(t *testing.T) {
	many := make([]map[string]any, 0, modelListLimit+10)
	for i := range modelListLimit + 10 {
		many = append(many, map[string]any{"name": fmt.Sprintf("m%d:latest", i)})
	}
	lister := listerFor(t, ProviderConfig{Provider: providerOllama}, "/api/tags",
		map[string]any{"models": many})
	models, err := lister.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != modelListLimit {
		t.Fatalf("returned %d models, want the cap of %d", len(models), modelListLimit)
	}
}

// A vendor's redirect must not carry the customer's credential to whatever host
// answered it.
//
// Go strips the headers it KNOWS are sensitive across a host change —
// Authorization, Cookie — and has never heard of `x-api-key`. Anthropic
// authenticates with exactly that, so a client that followed the hop would hand
// the key to the redirect target. Two servers, because one cannot show that the
// second never saw it.
func TestAListRedirectCarriesNoCredentialToTheNewHost(t *testing.T) {
	var landedHeaders http.Header
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		landedHeaders = r.Header.Clone()
		if err := json.NewEncoder(w).Encode(map[string]any{"data": []any{}}); err != nil {
			t.Errorf("encoding fixture: %v", err)
		}
	}))
	t.Cleanup(target.Close)

	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/v1/models", http.StatusFound)
	}))
	t.Cleanup(vendor.Close)

	client, err := SelectBrain(
		ProviderConfig{Provider: providerAnthropic, BaseURL: vendor.URL},
		cloudKeyFor(providerAnthropic, "sk-must-not-travel"),
	)
	if err != nil {
		t.Fatal(err)
	}
	// The hop is refused, so the 302 IS the answer and the caller reads it as
	// "this vendor did not answer" — which is the honest reading of a model
	// list endpoint that will not serve one.
	if _, err := client.(model.Lister).ListModels(context.Background()); err == nil {
		t.Fatal("a redirect was followed instead of refused")
	}
	if landedHeaders != nil {
		t.Fatalf("the redirect target was reached at all, with headers %v", landedHeaders)
	}
}
