// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The wire mapping, which is where a binding loses a field silently. A dropped
// base_url or input list does not fail anything — it routes to a different
// endpoint, or refuses a document the operator bound a model to carry.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestABindingSurvivesTheRoundTripToTheWireAndBack(t *testing.T) {
	original := ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierPremium: {
				Provider: "gemini", Model: "gemini-3.5-flash",
				BaseURL: "https://eu-gateway.example", Input: []string{"text", "image"},
			},
			ai.TierCheapCloud: {Provider: "gemini", Model: "gemini-3.1-flash-lite"},
			ai.TierFrontier:   {Provider: "gemini_vertex", Model: "gemini-3.5-flash", Location: "europe-west4"},
		},
		Embeddings: ai.EmbeddingsConfig{
			ProviderConfig: ai.ProviderConfig{Provider: "gemini_vertex", Model: "gemini-embedding-001", Location: "eu"},
			Dimensions:     1536,
		},
	}

	back := roundTrip(t, original)

	if back.Profile != original.Profile {
		t.Errorf("profile = %q, want %q", back.Profile, original.Profile)
	}
	if len(back.Tiers) != len(original.Tiers) {
		t.Fatalf("tiers = %v, want %d of them", back.Tiers, len(original.Tiers))
	}
	for tier, want := range original.Tiers {
		got := back.Tiers[tier]
		if got.Provider != want.Provider || got.Model != want.Model || got.BaseURL != want.BaseURL || got.Location != want.Location {
			t.Errorf("tier %s = %+v, want %+v", tier, got, want)
		}
		if len(got.Input) != len(want.Input) {
			t.Errorf("tier %s input = %v, want %v — a narrowed carriage that vanishes silently widens what the model may be sent", tier, got.Input, want.Input)
		}
	}
	if back.Embeddings.Dimensions != original.Embeddings.Dimensions {
		t.Errorf("embeddings width = %d, want %d", back.Embeddings.Dimensions, original.Embeddings.Dimensions)
	}
	if back.Embeddings.Location != original.Embeddings.Location {
		t.Errorf("embeddings location = %q, want %q — a gemini_vertex lane without it is refused on save", back.Embeddings.Location, original.Embeddings.Location)
	}
}

// Absent and empty are different to an operator and the same to a Go zero
// value. "No base_url override" must not read back as "base_url set to the
// empty string", which is why these fields are pointers on the wire.
func TestAnUnsetOptionalIsAbsentRatherThanEmpty(t *testing.T) {
	wire := mustWire(t, ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierPremium: {Provider: "fake", Model: "m"}},
	})
	tier := wire.Tiers["premium"]
	if tier.BaseUrl != nil {
		t.Errorf("base_url = %q, want absent", *tier.BaseUrl)
	}
	if tier.Input != nil {
		t.Errorf("input = %v, want absent", *tier.Input)
	}
	if tier.Location != nil {
		t.Errorf("location = %q, want absent on a binding that is not gemini_vertex", *tier.Location)
	}
	// Reported as STORED, not as defaulted: a GET → PUT round-trip must not
	// freeze today's compiled default into the document as though an operator
	// had chosen it, because then tomorrow's default would not reach them.
	if wire.Embeddings.Dimensions != nil {
		t.Errorf("dimensions = %d, want absent for an unset width", *wire.Embeddings.Dimensions)
	}
}

// Three states, and the difference between the last two is an operator's
// choice: absent is the product default, `{}` is the broker's own routing. A
// round trip that turns absent into `{}` silently re-routes every call, and one
// that drops a false pointer turns "do not" into the broker's default.
func TestABindingsRoutingSurvivesTheRoundTripInAllThreeStates(t *testing.T) {
	no := false
	cases := map[string]*ai.OpenRouterRouting{
		"absent": nil,
		"empty":  {},
		"populated": {
			Provider:  ai.OpenRouterProvider{Only: []string{"deepinfra"}, RequireParameters: &no, AllowFallbacks: &no},
			Reasoning: &ai.OpenRouterReasoning{Effort: "low"},
		},
	}
	for label, routing := range cases {
		openRouter := ai.ProviderConfig{
			Provider: "openai_compatible", Model: "openai/gpt-oss-120b",
			BaseURL: "https://openrouter.ai/api", Routing: routing,
		}
		original := ai.RoutingConfig{
			Profile:    ai.ProfileEUHosted,
			Tiers:      map[ai.Tier]ai.ProviderConfig{ai.TierCheapCloud: openRouter},
			Embeddings: ai.EmbeddingsConfig{ProviderConfig: openRouter},
		}
		back := roundTrip(t, original)
		if got := back.Tiers[ai.TierCheapCloud].Routing; !reflect.DeepEqual(got, routing) {
			t.Errorf("%s: tier routing came back as %#v, want %#v", label, got, routing)
		}
		if got := back.Embeddings.Routing; !reflect.DeepEqual(got, routing) {
			t.Errorf("%s: embeddings routing came back as %#v, want %#v", label, got, routing)
		}
	}
}

// Every field of a routing document reaches the wire and comes back. The
// fixture is checked for completeness first, by reflection: a field added to
// RoutingConfig, ProviderConfig, EmbeddingsConfig or OpenRouterRouting and left
// out of the fixture fails here, rather than passing a round trip that never
// exercised it — which is how a whole preferences block once went missing on
// every admin save without a test noticing.
//
// The mapping validates nothing, so the fixture need not be a binding the
// store would accept: a thinking_level or a location on an openai_compatible
// lane, and on the embeddings lane, is refused there, and must still reach it
// to be refused.
func TestEveryRoutingFieldSurvivesTheRoundTrip(t *testing.T) {
	yes, half, budget := true, 0.5, 512
	openRouter := ai.ProviderConfig{
		Provider: "openai_compatible", Model: "m", BaseURL: "https://openrouter.ai/api",
		Location: "eu", Input: []string{"text", "image"}, ThinkingLevel: "medium",
		Routing: &ai.OpenRouterRouting{
			Provider: ai.OpenRouterProvider{
				Only: []string{"a"}, Ignore: []string{"b"}, Quantizations: []string{"bf16"},
				Sort: &ai.OpenRouterSort{By: "latency", Partition: "none"}, RequireParameters: &yes, AllowFallbacks: &yes,
				PreferredMaxLatency: &ai.OpenRouterPctile{P50: &half, P75: &half, P90: &half, P99: &half},
				Order:               []string{"c"}, DataCollection: "deny", ZDR: &yes, EnforceDistillableText: &yes,
				MaxPrice:               &ai.OpenRouterPrice{Prompt: &half, Completion: &half, Request: &half, Image: &half},
				PreferredMinThroughput: &ai.OpenRouterPctile{All: &half},
			},
			Reasoning: &ai.OpenRouterReasoning{Effort: "low", MaxTokens: &budget, Exclude: &yes, Enabled: &yes},
		},
	}
	full := ai.RoutingConfig{
		Profile:    ai.ProfileEUHosted,
		Tiers:      map[ai.Tier]ai.ProviderConfig{ai.TierPremium: openRouter},
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: openRouter, Dimensions: 768},
		Decisions:  &ai.DecisionsConfig{Provider: "jev_compatible", Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api/alpha/decisions"},
		Providers: map[string]ai.ProviderSettings{
			"openai_compatible": {
				BaseURL: "https://openrouter.ai/api",
				Upstream: &ai.OpenRouterRouting{Provider: ai.OpenRouterProvider{
					Only: []string{"a"}, Ignore: []string{"b"}, AllowFallbacks: &yes,
					ZDR: &yes, DataCollection: "deny", EnforceDistillableText: &yes,
				}},
				Location: "eu",
			},
		},
	}
	// A provider's upstream carries the connection keys alone; the store
	// refuses the rest there, so the wire has no field for them. A threshold is
	// one number or a set of percentiles, never both.
	exempt := map[string]bool{"RoutingConfig.Providers[openai_compatible].Upstream.Reasoning": true}
	for _, field := range []string{"Quantizations", "Sort", "RequireParameters", "PreferredMaxLatency", "Order", "MaxPrice", "PreferredMinThroughput"} {
		exempt["RoutingConfig.Providers[openai_compatible].Upstream.Provider."+field] = true
	}
	for _, lane := range []string{"Tiers[premium]", "Embeddings.ProviderConfig"} {
		exempt["RoutingConfig."+lane+".Routing.Provider.PreferredMaxLatency.All"] = true
		for _, p := range []string{"P50", "P75", "P90", "P99"} {
			exempt["RoutingConfig."+lane+".Routing.Provider.PreferredMinThroughput."+p] = true
		}
	}
	// Reasoning takes effort or max_tokens; the mapping carries both regardless.
	assertEveryFieldSet(t, reflect.ValueOf(full), "RoutingConfig", exempt)

	back := roundTrip(t, full)
	if !reflect.DeepEqual(back, full) {
		t.Errorf("routing came back as %#v, want %#v", back, full)
	}
}

// GET shows each lane the host it is served at, read from its provider, so a
// client that predates `providers` still sees where a lane goes; the embeddings
// lane's own server wins for that lane alone.
func TestEachLaneReadsItsProvidersHostOnTheWire(t *testing.T) {
	cfg := ai.RoutingConfig{
		Profile: ai.ProfileCloudFrontier,
		Tiers:   map[ai.Tier]ai.ProviderConfig{ai.TierPremium: {Provider: "openai_compatible", Model: "m"}},
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: ai.ProviderConfig{
			Provider: "openai_compatible", Model: "e", BaseURL: "https://embed.example",
		}},
		Decisions: &ai.DecisionsConfig{Provider: "jev_compatible", Model: "d"},
		Providers: map[string]ai.ProviderSettings{
			"openai_compatible": {BaseURL: "https://openrouter.ai/api"},
			"jev_compatible":    {BaseURL: "https://openrouter.ai/api/alpha/decisions"},
		},
	}

	wire := mustWire(t, cfg)

	if got := deref(wire.Tiers[string(ai.TierPremium)].BaseUrl); got != "https://openrouter.ai/api" {
		t.Errorf("premium base_url = %q, want the provider's host", got)
	}
	if got := deref(wire.Embeddings.BaseUrl); got != "https://embed.example" {
		t.Errorf("embeddings base_url = %q, want its own server", got)
	}
	if got := deref(wire.Decisions.BaseUrl); got != "https://openrouter.ai/api/alpha/decisions" {
		t.Errorf("decisions base_url = %q, want the provider's endpoint", got)
	}
	if wire.Providers == nil || deref((*wire.Providers)["openai_compatible"].BaseUrl) != "https://openrouter.ai/api" {
		t.Errorf("providers = %#v, want the openai_compatible host", wire.Providers)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// An unbound decision lane stays unbound across a save. A round trip that
// turned absent into an empty binding would hand every decision site a lane
// with no provider, where absent sends it straight to its LLM ladder.
func TestAnAbsentDecisionLaneStaysAbsent(t *testing.T) {
	wire := mustWire(t, ai.RoutingConfig{Profile: ai.ProfileEUHosted})
	if wire.Decisions != nil {
		t.Fatalf("decisions = %+v on the wire, want absent", *wire.Decisions)
	}
	if back := mustConfig(t, wire); back.Decisions != nil {
		t.Errorf("decisions = %+v after the round trip, want absent", *back.Decisions)
	}
}

// assertEveryFieldSet walks a fixture and fails on any exported field left at
// its zero value, descending through pointers, structs and map values, except
// the paths in exempt, which the wire deliberately has no field for. Only
// exported fields are configuration a document carries; the unexported ones
// (a source digest, a key resolver) are stamped by whoever loaded the config
// and have no wire spelling to lose.
func assertEveryFieldSet(t *testing.T, v reflect.Value, path string, exempt map[string]bool) {
	t.Helper()
	if exempt[path] {
		return
	}
	if v.IsZero() {
		t.Errorf("%s is unset in the fixture — set it so the round trip exercises it", path)
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		assertEveryFieldSet(t, v.Elem(), path, exempt)
	case reflect.Map:
		for _, key := range v.MapKeys() {
			assertEveryFieldSet(t, v.MapIndex(key), path+"["+key.String()+"]", exempt)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if field := v.Type().Field(i); field.IsExported() {
				assertEveryFieldSet(t, v.Field(i), path+"."+field.Name, exempt)
			}
		}
	}
}

func roundTrip(t *testing.T, cfg ai.RoutingConfig) ai.RoutingConfig {
	t.Helper()
	return mustConfig(t, mustWire(t, cfg))
}

func mustWire(t *testing.T, cfg ai.RoutingConfig) crmcontracts.AiRouting {
	t.Helper()
	wire, err := toContractAiRouting(cfg)
	if err != nil {
		t.Fatalf("mapping to the wire: %v", err)
	}
	return wire
}

func mustConfig(t *testing.T, wire crmcontracts.AiRouting) ai.RoutingConfig {
	t.Helper()
	cfg, err := fromContractAiRouting(wire, nil)
	if err != nil {
		t.Fatalf("mapping from the wire: %v", err)
	}
	return cfg
}

// An unbound installation answers `{}`, never null. A null leaves a client
// unable to tell "nothing is bound" from "the field was omitted".
func TestAnUnboundInstallationReportsAnEmptyTierMapNotNull(t *testing.T) {
	wire := mustWire(t, ai.RoutingConfig{})
	if wire.Tiers == nil {
		t.Error("tiers is null; an unbound installation must say so with an empty object")
	}
	if len(wire.Tiers) != 0 {
		t.Errorf("tiers = %v, want empty", wire.Tiers)
	}
}

// A submitted document with no tiers maps to the unconfigured config, which is
// what lets an operator unbind every model deliberately.
func TestASubmittedDocumentWithNoTiersIsUnconfigured(t *testing.T) {
	cfg := mustConfig(t, crmcontracts.AiRouting{Profile: "eu_hosted"})
	if !cfg.Unconfigured() {
		t.Errorf("tiers = %v, want unconfigured", cfg.Tiers)
	}
}

// agentReq is a request carrying an AGENT principal — a passport-authenticated
// caller rather than a signed-in human.
func agentReq(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/routing", strings.NewReader(body))
	ctx := principal.WithWorkspaceID(req.Context(), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + ids.NewV7().String(),
		Permissions: principal.Permissions{
			// Deliberately granted the object. The refusal under test must not
			// depend on the agent lacking the grant — an agent could hold one
			// through a passport whose scopes admit it.
			Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	return req.WithContext(ctx)
}

// An agent never re-points which vendor processes the installation's
// correspondence, WHATEVER its passport scopes admit. This is the governance
// claim the contract makes with x-agent-access: human-only, and it must hold at
// the handler rather than only in the document.
func TestAnAgentCannotReplaceTheModelBinding(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()

	h.ReplaceAiRouting(rec, agentReq(`{"profile":"eu_hosted","tiers":{}}`))

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d — an agent re-pointed the model binding", rec.Code, http.StatusForbidden)
	}
}

func TestAnAgentCannotSetAProvidersHost(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()

	h.SetAiProviderSettings(rec, agentReq(`{"base_url":"https://evil.example"}`), "openai_compatible")

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d — an agent re-pointed a provider", rec.Code, http.StatusForbidden)
	}
}

// A malformed body is a 422 naming the fault, not a panic and not a partially
// applied binding.
func TestAMalformedBindingDocumentIsRefused(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/routing", strings.NewReader("{not json"))
	ctx := principal.WithWorkspaceID(req.Context(), ids.NewV7())
	req = req.WithContext(principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	}))

	h.ReplaceAiRouting(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want %d for a body that is not JSON", rec.Code, http.StatusUnprocessableEntity)
	}
}

// A role that wired no store answers 501, not a nil dereference. Both verbs,
// because a role wires them together and a half-wired surface is the state
// nobody would think to check.
func TestAnUnwiredRoutingSurfaceIsNotImplemented(t *testing.T) {
	var h aiRoutingHandlers
	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"GetAiRouting":     h.GetAiRouting,
		"ReplaceAiRouting": h.ReplaceAiRouting,
		"SetAiProviderSettings": func(w http.ResponseWriter, r *http.Request) {
			h.SetAiProviderSettings(w, r, "openai_compatible")
		},
	} {
		rec := httptest.NewRecorder()
		call(rec, httptest.NewRequest(http.MethodGet, "/v1/ai/routing", nil))
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("%s: status = %d, want %d", name, rec.Code, http.StatusNotImplemented)
		}
	}
}

func locationsReq(ctx context.Context) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/v1/ai/provider-locations/gemini_vertex", nil).WithContext(ctx)
}

// The location list is human-only like the model list: an agent is refused
// whatever its grant, and so is a human without ai_routing:read.
func TestTheLocationListIsForAHumanHoldingTheRoutingReadGrant(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}

	agent := httptest.NewRecorder()
	h.ListProviderLocations(agent, agentReq(""), "gemini_vertex")
	if agent.Code != http.StatusForbidden {
		t.Errorf("agent: status = %d, want %d", agent.Code, http.StatusForbidden)
	}

	unentitled := httptest.NewRecorder()
	h.ListProviderLocations(unentitled, locationsReq(routingSeat(principal.ObjectGrant{})), "gemini_vertex")
	if unentitled.Code != http.StatusForbidden {
		t.Errorf("no ai_routing:read: status = %d, want %d", unentitled.Code, http.StatusForbidden)
	}

	unwired := httptest.NewRecorder()
	aiRoutingHandlers{}.ListProviderLocations(unwired, locationsReq(routingSeat(principal.ObjectGrant{Read: true})), "gemini_vertex")
	if unwired.Code != http.StatusNotImplemented {
		t.Errorf("unwired: status = %d, want %d", unwired.Code, http.StatusNotImplemented)
	}
}

// A vendor with no location to choose answers 200 with the reason and an
// empty array, never null.
func TestAVendorWithNoLocationsSaysSo(t *testing.T) {
	h := aiRoutingHandlers{store: &ai.RoutingStore{}}
	rec := httptest.NewRecorder()
	h.ListProviderLocations(rec, locationsReq(routingSeat(principal.ObjectGrant{Read: true})), "anthropic")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"locations":[],"provider":"anthropic","unavailable":"not_published"}` {
		t.Errorf("body = %s", body)
	}
}

func routingSeat(grant principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"ai_routing": grant},
			RowScope: principal.RowScopeAll,
		},
	})
}
