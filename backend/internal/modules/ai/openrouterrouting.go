// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// OpenRouterRouting is the upstream-selection preferences for a broker on the
// OpenAI wire.
//
// A gateway fronting many inference hosts chooses one per request, and its
// default choice optimizes PRICE: stable hosts first, then weighted by the
// inverse square of cost. That is the opposite of what this product wants. The
// same model id is served by hosts differing in quantization (fp4 through
// bf16), output ceiling (8k through 118k tokens) and tail latency, so the
// unpinned default makes answer quality and response time a per-request
// lottery — and one nobody can see, because the broker reports the upstream in
// a field this wire's callers rarely read.
//
// These preferences are how a deployment says which trade it wants. They are
// deliberately NOT yaml-visible yet: the field set that matters is being
// measured before it becomes a surface an operator can depend on.
// The json tags are the ENV VAR's spelling, not a config file's: MARGINCE_AICERT_UPSTREAM
// carries one of these as JSON and refuses unknown keys, so an operator types
// `reasoning_effort` rather than Go's ReasoningEffort. Without the tags the
// refusal would reject exactly the snake_case names the reference documents.
type OpenRouterRouting struct {
	// Only and Ignore are an allowlist and a blocklist of upstream slugs. Both
	// are HARD filters — an excluded host is removed from the candidate set,
	// not merely deprioritized — which makes them the only way to bound the
	// tail rather than hope for it.
	//
	// A base slug matches every variant and region of a host; a full slug
	// ("deepinfra/turbo") pins one endpoint.
	Only   []string `json:"only"`
	Ignore []string `json:"ignore"`
	// Quantizations restricts serving precision (bf16, fp16, fp8, fp4, int8 …).
	// Also a hard filter, and the one that decides whether repeated calls are
	// COMPARABLE: two answers from the same model id at different precision are
	// two different models for every purpose except billing.
	Quantizations []string `json:"quantizations"`
	// Sort orders candidates by "price", "throughput" or "latency". It reorders
	// rather than filters, and setting it disables the broker's load balancing
	// entirely — so it buys a consistent preference at the cost of spreading
	// load, which is a trade to make deliberately.
	Sort string `json:"sort"`
	// RequireParameters keeps the request away from hosts that do not support
	// every parameter it carries. Soft preferences already apply for tools and
	// response_format, so this is belt-and-braces for a structured-output call
	// rather than the thing that makes one work — but it turns a preference the
	// broker may weigh into a guarantee it must honour.
	RequireParameters bool `json:"require_parameters"`
	// AllowFallbacks, when non-nil, overrides the broker's default of switching
	// hosts on failure. A pointer because false is a real choice and the zero
	// value must not be mistaken for it: turning fallbacks off trades
	// availability for the certainty of being served by one host, which only a
	// compliance rule or a measurement run should ask for.
	AllowFallbacks *bool `json:"allow_fallbacks"`
	// PreferredMaxLatencyP90 deprioritizes hosts whose 90th-percentile latency
	// over a rolling five-minute window exceeds this many seconds. SOFT: a host
	// past the threshold is moved down the list, never removed, so this alone
	// cannot bound the tail — pair it with Only or Quantizations when the tail
	// is what matters. 0 leaves it unset.
	PreferredMaxLatencyP90 float64 `json:"preferred_max_latency_p90"`
	// ReasoningEffort caps a reasoning model's thinking budget ("none",
	// "minimal", "low", "medium", "high", "xhigh", "max"). Unset means each
	// host applies its OWN default, which is why it belongs here: thinking is
	// charged to the same output budget as the answer, so an uncontrolled
	// default varies cost, latency and — when it exhausts the budget before the
	// answer starts — whether there is an answer at all.
	ReasoningEffort string `json:"reasoning_effort"`
}

// empty reports whether these preferences would add nothing to a request, so
// the wire carries no `provider` object at all rather than an object of
// defaults that would disable load balancing by accident.
func (r *OpenRouterRouting) empty() bool {
	if r == nil {
		return true
	}
	return len(r.Only) == 0 && len(r.Ignore) == 0 && len(r.Quantizations) == 0 &&
		r.Sort == "" && !r.RequireParameters && r.AllowFallbacks == nil &&
		r.PreferredMaxLatencyP90 == 0
}

// openAICompatProviderWire is the broker's `provider` object. Every field is
// omitempty: a broker reads an explicitly-null preference as a preference.
type openAICompatProviderWire struct {
	Only                []string                       `json:"only,omitempty"`
	Ignore              []string                       `json:"ignore,omitempty"`
	Quantizations       []string                       `json:"quantizations,omitempty"`
	Sort                string                         `json:"sort,omitempty"`
	RequireParameters   bool                           `json:"require_parameters,omitempty"`
	AllowFallbacks      *bool                          `json:"allow_fallbacks,omitempty"`
	PreferredMaxLatency *openAICompatLatencyPercentile `json:"preferred_max_latency,omitempty"`
}

// openAICompatLatencyPercentile is the percentile-keyed threshold shape the
// broker takes; p90 is the percentile a user-facing path is judged on.
type openAICompatLatencyPercentile struct {
	P90 float64 `json:"p90"`
}

// openAICompatReasoningWire is the reasoning-model control block.
type openAICompatReasoningWire struct {
	Effort string `json:"effort,omitempty"`
}

// providerWire renders the `provider` object, or nil when these preferences
// would say nothing.
func (r *OpenRouterRouting) providerWire() *openAICompatProviderWire {
	if r.empty() {
		return nil
	}
	wire := &openAICompatProviderWire{
		Only: r.Only, Ignore: r.Ignore, Quantizations: r.Quantizations,
		Sort: r.Sort, RequireParameters: r.RequireParameters, AllowFallbacks: r.AllowFallbacks,
	}
	if r.PreferredMaxLatencyP90 > 0 {
		wire.PreferredMaxLatency = &openAICompatLatencyPercentile{P90: r.PreferredMaxLatencyP90}
	}
	return wire
}

// reasoningWire renders the `reasoning` block, or nil when no effort is set —
// leaving the host's own default in place, which is the pre-existing behaviour.
func (r *OpenRouterRouting) reasoningWire() *openAICompatReasoningWire {
	if r == nil || r.ReasoningEffort == "" {
		return nil
	}
	return &openAICompatReasoningWire{Effort: r.ReasoningEffort}
}
