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
