// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

// A binding that names no preference must put NO `provider` object on the wire.
//
// This is the case worth a test rather than the interesting one: an object of
// zero values looks harmless and is not. Setting `sort` — even to "" — turns
// off the broker's load balancing, so a request that meant to say nothing would
// change how every one of its calls is routed.
func TestNoPreferencesPutNoProviderObjectOnTheWire(t *testing.T) {
	for name, routing := range map[string]*OpenRouterRouting{
		"a binding with no routing at all": nil,
		"a routing that names nothing":     {},
	} {
		t.Run(name, func(t *testing.T) {
			if got := routing.providerWire(); got != nil {
				t.Errorf("providerWire() = %+v, want nil", got)
			}
			if got := routing.reasoningWire(); got != nil {
				t.Errorf("reasoningWire() = %+v, want nil", got)
			}
			body, err := json.Marshal(openAICompatChatWire{
				Model: "m", Provider: routing.providerWire(), Reasoning: routing.reasoningWire(),
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{`"provider"`, `"reasoning"`} {
				if strings.Contains(string(body), key) {
					t.Errorf("body carries %s with no preference set: %s", key, body)
				}
			}
		})
	}
}

// Reasoning effort travels on its own, without dragging a provider object with
// it: capping the thinking budget is not a statement about which host serves.
func TestReasoningEffortAloneCarriesNoProviderObject(t *testing.T) {
	routing := &OpenRouterRouting{ReasoningEffort: "low"}
	if got := routing.providerWire(); got != nil {
		t.Errorf("providerWire() = %+v, want nil — effort says nothing about upstream selection", got)
	}
	reasoning := routing.reasoningWire()
	if reasoning == nil || reasoning.Effort != "low" {
		t.Fatalf("reasoningWire() = %+v, want effort low", reasoning)
	}
}

// The percentile threshold is nested rather than flat on the wire, and is
// omitted entirely when unset — a `preferred_max_latency` of {"p90":0} would
// ask the broker to prefer hosts faster than zero seconds.
func TestPreferredLatencyIsOmittedUntilItIsSet(t *testing.T) {
	unset, err := json.Marshal((&OpenRouterRouting{Sort: "throughput"}).providerWire())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unset), "preferred_max_latency") {
		t.Errorf("unset threshold reached the wire: %s", unset)
	}
	set, err := json.Marshal((&OpenRouterRouting{PreferredMaxLatencyP90: 8}).providerWire())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(set), `"preferred_max_latency":{"p90":8}`) {
		t.Errorf("threshold rendered as %s", set)
	}
}

// allow_fallbacks is a pointer because false is a real choice: turning fallbacks
// off trades availability for being served by one host. An unset field must not
// read as that choice.
func TestAllowFallbacksDistinguishesUnsetFromFalse(t *testing.T) {
	no := false
	for name, tc := range map[string]struct {
		routing *OpenRouterRouting
		wantKey bool
	}{
		"unset stays off the wire":  {&OpenRouterRouting{Sort: "price"}, false},
		"an explicit false is sent": {&OpenRouterRouting{AllowFallbacks: &no}, true},
	} {
		t.Run(name, func(t *testing.T) {
			body, err := json.Marshal(tc.routing.providerWire())
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(body), "allow_fallbacks"); got != tc.wantKey {
				t.Errorf("allow_fallbacks present = %v, want %v: %s", got, tc.wantKey, body)
			}
		})
	}
}

// The env var's spelling is the documented snake_case, and unknown keys are
// refused — a misspelt preference dropped in silence would let a tuned run
// report the untuned baseline's numbers under a tuned run's name.
func TestTheDocumentedPreferenceSpellingParses(t *testing.T) {
	const raw = `{"only":["cerebras"],"ignore":["novita"],"quantizations":["bf16","fp16"],` +
		`"sort":"throughput","require_parameters":true,"allow_fallbacks":false,` +
		`"preferred_max_latency_p90":8,"reasoning_effort":"low"}`
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var routing OpenRouterRouting
	if err := decoder.Decode(&routing); err != nil {
		t.Fatalf("the spelling the reference documents must parse: %v", err)
	}
	if routing.ReasoningEffort != "low" || routing.Sort != "throughput" || routing.PreferredMaxLatencyP90 != 8 {
		t.Errorf("decoded %+v", routing)
	}
	if routing.AllowFallbacks == nil || *routing.AllowFallbacks {
		t.Error("an explicit allow_fallbacks:false must survive as a set false, not an unset nil")
	}

	decoder = json.NewDecoder(strings.NewReader(`{"quantisations":["bf16"]}`))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&OpenRouterRouting{}); err == nil {
		t.Error("a misspelt key must be refused, not dropped: a tuned run would report baseline numbers")
	}
}

// A broker binding that declares nothing comes out of the parser carrying the
// reliability-over-price default; a direct vendor on the same OpenAI wire comes
// out carrying nothing at all.
//
// The second half is the one worth a test. `openai_compatible` also serves
// direct vendors named by base_url, and no vendor is obliged to ignore an
// unknown top-level key politely — so defaulting on the provider name rather
// than the host would send Mistral a `provider` object it never asked for.
func TestTheUpstreamDefaultReachesTheBrokerAndNobodyElse(t *testing.T) {
	for name, tc := range map[string]struct {
		provider, baseURL string
		wantDefault       bool
	}{
		"the broker":                  {providerOpenAICompatible, "https://openrouter.ai/api", true},
		"a broker subdomain":          {providerOpenAICompatible, "https://gateway.openrouter.ai", true},
		"a direct vendor on the wire": {providerOpenAICompatible, "https://api.mistral.ai", false},
		// The lookalike a substring test would have accepted as the broker.
		"a lookalike host": {providerOpenAICompatible, "https://openrouter.ai.evil.test", false},
		"a native vendor":  {providerGemini, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := RoutingConfig{
				Profile: ProfileEUHosted,
				Tiers: map[Tier]ProviderConfig{
					TierCheapCloud: {Provider: tc.provider, Model: "m", BaseURL: tc.baseURL},
				},
			}
			cfg.applyUpstreamDefaults()
			got := cfg.Tiers[TierCheapCloud].Routing
			if tc.wantDefault {
				if got == nil {
					t.Fatal("the broker binding inherited no preferences")
				}
				if got.Sort != SortThroughput || !got.RequireParameters || len(got.Quantizations) == 0 {
					t.Errorf("inherited %+v, want reliability over price", got)
				}
				return
			}
			if got != nil {
				t.Errorf("inherited %+v; only an OpenRouter host may be sent OpenRouter's own fields", got)
			}
		})
	}
}

// An operator who writes an empty block is opting out, and the default must not
// overwrite that — otherwise the opt-out is unwritable and the setting round-trips
// back to the default on the next read.
func TestAnExplicitlyEmptyDeclarationIsNotRedefaulted(t *testing.T) {
	cfg := RoutingConfig{
		Profile: ProfileEUHosted,
		Tiers: map[Tier]ProviderConfig{
			TierCheapCloud: {
				Provider: providerOpenAICompatible, Model: "m",
				BaseURL: "https://openrouter.ai/api",
				Routing: &OpenRouterRouting{},
			},
		},
	}
	cfg.applyUpstreamDefaults()
	got := cfg.Tiers[TierCheapCloud].Routing
	if got == nil {
		t.Fatal("the declaration went missing")
	}
	if !got.IsEmpty() {
		t.Errorf("an opt-out was overwritten with %+v", got)
	}
}

// A preference the broker would silently drop is refused at parse time instead.
//
// Silence is the reason: the broker ignores an unknown sort rather than
// rejecting it, so a deployment would run price-weighted while its config said
// otherwise — and the only symptom would be the latency the setting exists to
// remove.
func TestAPreferenceTheBrokerWouldIgnoreIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		routing *OpenRouterRouting
		wantErr string
	}{
		"an unknown sort":            {&OpenRouterRouting{Sort: "cheapest"}, "sort"},
		"an unknown quantization":    {&OpenRouterRouting{Quantizations: []string{"fp5"}}, "quantization"},
		"an unknown effort":          {&OpenRouterRouting{ReasoningEffort: "lots"}, "reasoning_effort"},
		"a negative latency ceiling": {&OpenRouterRouting{PreferredMaxLatencyP90: -1}, "negative"},
		"the accepted vocabulary":    {DefaultOpenRouterRouting(), ""},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.routing.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("the default must validate: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("accepted %+v, which the broker would drop in silence", tc.routing)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name %q, so an operator cannot tell which field to fix", err, tc.wantErr)
			}
		})
	}
}

// A block on a binding that cannot honour it is refused, and the refusal names
// which of the two reasons applies — the provider or the host — because those
// are different edits.
func TestARoutingBlockIsRefusedWhereItCannotApply(t *testing.T) {
	for name, tc := range map[string]struct {
		binding ProviderConfig
		wantErr string
	}{
		"a native vendor": {
			ProviderConfig{Provider: providerGemini, Model: "m", Routing: &OpenRouterRouting{Sort: SortThroughput}},
			"serves one model from one host",
		},
		"the OpenAI wire pointed elsewhere": {
			ProviderConfig{Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://api.mistral.ai", Routing: &OpenRouterRouting{Sort: SortThroughput}},
			"is not an OpenRouter host",
		},
		"the broker itself": {
			ProviderConfig{Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://openrouter.ai/api", Routing: DefaultOpenRouterRouting()},
			"",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := validateUpstreamPreferences("cheap_cloud", tc.binding)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("refused a legal binding: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("accepted a block that would have been sent to a host that never asked for it")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not say why it cannot apply", err)
			}
		})
	}
}
