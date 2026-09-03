// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
)

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
// The yaml and json spellings are deliberately the same string: an operator
// writes `reasoning_effort` in the routing config, the settings store round-trips
// the identical key as JSON, and MARGINCE_AICERT_UPSTREAM takes one of these as
// JSON too. One name per field across all three, so a preference cannot mean a
// different thing depending on which door it came through.
type OpenRouterRouting struct {
	// Only and Ignore are an allowlist and a blocklist of upstream slugs. Both
	// are HARD filters — an excluded host is removed from the candidate set,
	// not merely deprioritized — which makes them the only way to bound the
	// tail rather than hope for it.
	//
	// A base slug matches every variant and region of a host; a full slug
	// ("deepinfra/turbo") pins one endpoint.
	Only   []string `yaml:"only" json:"only"`
	Ignore []string `yaml:"ignore" json:"ignore"`
	// Quantizations restricts serving precision (bf16, fp16, fp8, fp4, int8 …).
	// Also a hard filter, and the one that decides whether repeated calls are
	// COMPARABLE: two answers from the same model id at different precision are
	// two different models for every purpose except billing.
	Quantizations []string `yaml:"quantizations" json:"quantizations"`
	// Sort orders candidates by "price", "throughput" or "latency". It reorders
	// rather than filters, and setting it disables the broker's load balancing
	// entirely — so it buys a consistent preference at the cost of spreading
	// load, which is a trade to make deliberately.
	Sort string `yaml:"sort" json:"sort"`
	// RequireParameters keeps the request away from hosts that do not support
	// every parameter it carries. Soft preferences already apply for tools and
	// response_format, so this is belt-and-braces for a structured-output call
	// rather than the thing that makes one work — but it turns a preference the
	// broker may weigh into a guarantee it must honour.
	// A pointer for the same reason AllowFallbacks is one: false is a real
	// choice and must not read as unset. A plain bool loses it twice over —
	// omitempty drops it from the wire, and providerBlockEmpty would call a
	// block containing only `require_parameters: false` empty and emit no
	// provider object at all, so the operator's "do NOT require these" would
	// silently become the broker's own default.
	RequireParameters *bool `yaml:"require_parameters" json:"require_parameters"`
	// AllowFallbacks, when non-nil, overrides the broker's default of switching
	// hosts on failure. A pointer because false is a real choice and the zero
	// value must not be mistaken for it: turning fallbacks off trades
	// availability for the certainty of being served by one host, which only a
	// compliance rule or a measurement run should ask for.
	AllowFallbacks *bool `yaml:"allow_fallbacks" json:"allow_fallbacks"`
	// PreferredMaxLatencyP90 deprioritizes hosts whose 90th-percentile latency
	// over a rolling five-minute window exceeds this many seconds. SOFT: a host
	// past the threshold is moved down the list, never removed, so this alone
	// cannot bound the tail — pair it with Only or Quantizations when the tail
	// is what matters. 0 leaves it unset.
	PreferredMaxLatencyP90 float64 `yaml:"preferred_max_latency_p90" json:"preferred_max_latency_p90"`
	// ReasoningEffort caps a reasoning model's thinking budget ("none",
	// "minimal", "low", "medium", "high", "xhigh", "max"). Unset means each
	// host applies its OWN default, which is why it belongs here: thinking is
	// charged to the same output budget as the answer, so an uncontrolled
	// default varies cost, latency and — when it exhausts the budget before the
	// answer starts — whether there is an answer at all.
	ReasoningEffort string `yaml:"reasoning_effort" json:"reasoning_effort"`
}

// providerBlockEmpty reports whether these preferences would add nothing to the
// request's `provider` object, so the wire carries no such object at all rather
// than an object of defaults that would disable load balancing by accident.
//
// Not the same question as IsEmpty: ReasoningEffort is a preference and is
// deliberately not counted here, because it travels in its own `reasoning`
// block. A binding that caps thinking and says nothing about upstream selection
// must send the second block and not the first.
func (r *OpenRouterRouting) providerBlockEmpty() bool {
	if r == nil {
		return true
	}
	return len(r.Only) == 0 && len(r.Ignore) == 0 && len(r.Quantizations) == 0 &&
		r.Sort == "" && r.RequireParameters == nil && r.AllowFallbacks == nil &&
		r.PreferredMaxLatencyP90 == 0
}

// IsEmpty reports whether an operator wrote a declaration that asks for
// nothing — `routing: {}`, the explicit opt-out that takes the broker's own
// price-weighted routing.
//
// A method rather than a comparison against the zero value because the struct
// holds slices and so is not comparable, and because the answer is a product
// question ("did they opt out?") that should have one spelling rather than
// being re-derived field by field at each call site.
func (r *OpenRouterRouting) IsEmpty() bool {
	return r.providerBlockEmpty() && (r == nil || r.ReasoningEffort == "")
}

// openAICompatProviderWire is the broker's `provider` object. Every field is
// omitempty: a broker reads an explicitly-null preference as a preference.
type openAICompatProviderWire struct {
	Only                []string                       `json:"only,omitempty"`
	Ignore              []string                       `json:"ignore,omitempty"`
	Quantizations       []string                       `json:"quantizations,omitempty"`
	Sort                string                         `json:"sort,omitempty"`
	RequireParameters   *bool                          `json:"require_parameters,omitempty"`
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
	if r.providerBlockEmpty() {
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

// openRouterHost is the broker these preferences belong to.
//
// They are its parameters, not a general OpenAI-wire feature: `provider` and
// `quantizations` are fields it invented, and no vendor is obliged to ignore an
// unknown top-level key politely. The openai_compatible binding also serves
// direct vendors — a Mistral or a Together endpoint named by base_url — so the
// preferences are keyed on the HOST rather than on the provider name, and a
// binding pointed somewhere else neither receives them nor may declare them.
const openRouterHost = "openrouter.ai"

// IsOpenRouterHost reports whether baseURL names the broker whose
// upstream-selection preferences OpenRouterRouting describes.
//
// Parsed rather than substring-matched: a substring test would accept
// "openrouter.ai.evil.test" as the broker and send it a config it never asked
// for, and would miss a legitimate "https://OPENROUTER.AI" that URLs are
// case-insensitive in.
func IsOpenRouterHost(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	// The scheme is checked, not just the host: "//openrouter.ai/api" parses
	// with the right hostname and is not something the HTTP client can send on,
	// so treating it as the broker would apply the default to a binding that
	// cannot make a request at all. Shares the predicate with isFetchableURL.
	if !sendableHTTPScheme(parsed) {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == openRouterHost || strings.HasSuffix(host, "."+openRouterHost)
}

// DefaultOpenRouterRouting is what a broker binding gets when it declares no
// preferences of its own: reliability first, price second.
//
// The broker's own default is the opposite — it skips hosts that failed in the
// last 30 seconds and then weights by the INVERSE SQUARE of price, so the
// cheapest eligible host wins most calls. For a model served by 21 hosts that
// makes latency and answer quality a per-request lottery. Measured on this
// tree's certification corpus on 2026-09-02, unpinned against these three
// preferences, gpt-oss-120b on draft_reply: p50 19.0s → 1.1s, p90 38.0s → 2.0s,
// p99 304.2s → 3.7s, hosts reached 8 → 1, and the repeats of 8 of 9 scenarios
// stopped being split across different hosts. cold_start moved the same way.
// The full measurement is docs/reference/openrouter.md.
//
// Why these three and not the faster ones the same experiment found:
//
//   - sort: throughput is the lever. It is what collapses the tail; the other
//     two change almost nothing while it is set.
//   - quantizations pins SERVING PRECISION, which is what makes repeated calls
//     comparable at all — two answers from one model id at fp4 and at bf16 are
//     two different models for every purpose except billing. It buys nothing
//     measurable today and exists for the day the fastest host drops out, when
//     the sort would otherwise fall to an fp4 host and answer quality would
//     shift with nothing to show it.
//   - require_parameters keeps a structured-output request away from a host that
//     cannot honour response_format. A soft preference already covers this most
//     of the time; this makes it a rule.
//
// Deliberately absent: max_price, whose measured p99 was 387 seconds because a
// price ceiling cannot exclude what the sort then prefers;
// preferred_max_latency, which only reorders and cannot bound a tail; only/
// ignore, which pin by slug and throw away the failover breadth that survives
// here; and reasoning_effort, which halves latency and cost and costs a fifth
// of the certification score, so it belongs on a task that wants throughput and
// not in a drafting default. allow_fallbacks is left unset so the broker's own
// true stands: pinning the sort is not a reason to stop failing over.
func DefaultOpenRouterRouting() *OpenRouterRouting {
	requireParameters := true
	return &OpenRouterRouting{
		Sort:              SortThroughput,
		Quantizations:     []string{"fp16", "bf16"},
		RequireParameters: &requireParameters,
	}
}

// The sort orders the broker accepts. Named because two of them are traps a
// config could otherwise reach for: price is what the unpinned default already
// effectively does, and latency measured worse than throughput here (it reached
// Groq where throughput reached Cerebras).
const (
	SortThroughput = "throughput"
	SortPrice      = "price"
	SortLatency    = "latency"
)

// sortOrders and quantizationLevels are the accepted vocabularies, checked at
// parse time so a typo fails at boot rather than being dropped in silence by a
// broker that treats an unknown value as no value.
var (
	sortOrders = []string{SortThroughput, SortPrice, SortLatency}
	// The broker's published levels. `unknown` is one of them and is accepted:
	// several hosts genuinely report it, and refusing it would make a filter
	// that admits them unwritable.
	quantizationLevels = []string{
		"int4", "int8", "fp4", "mxfp4", "nvfp4", "fp6", "fp8", "mxfp8",
		"fp16", "bf16", "fp32", "unknown",
	}
	// The effort levels the broker accepts, hardest first.
	reasoningEfforts = []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"}
)

// Validate refuses a preference the broker would silently ignore.
//
// Exported because the config file is not the only door: the certification
// lane takes these preferences from an environment variable, and a run that
// accepted a misspelt value would report the untuned baseline under a tuned
// run's name. One check, both doors.
//
// Silence is the whole reason this exists: an unknown sort or quantization is
// dropped rather than rejected upstream, so a deployment would run with the
// price-weighted default while its config file said otherwise — and the only
// symptom would be the latency this default exists to remove.
func (r *OpenRouterRouting) Validate() error {
	if r == nil {
		return nil
	}
	if r.Sort != "" && !slices.Contains(sortOrders, r.Sort) {
		return fmt.Errorf("ai: routing config: sort %q is not one of %s", r.Sort, strings.Join(sortOrders, " | "))
	}
	// The generated schema declares these arrays minItems:1 and uniqueItems, so
	// the parser has to refuse the same shapes — an editor and a runtime that
	// authorize different configs is the drift the parity gate exists to catch,
	// and the weaker half is the one that decides what actually runs.
	for name, list := range map[string][]string{"only": r.Only, "ignore": r.Ignore, "quantizations": r.Quantizations} {
		if err := refuseEmptyOrRepeated(name, list); err != nil {
			return err
		}
	}
	for _, q := range r.Quantizations {
		if !slices.Contains(quantizationLevels, q) {
			return fmt.Errorf("ai: routing config: quantization %q is not one of %s", q, strings.Join(quantizationLevels, " | "))
		}
	}
	if r.ReasoningEffort != "" && !slices.Contains(reasoningEfforts, r.ReasoningEffort) {
		return fmt.Errorf("ai: routing config: reasoning_effort %q is not one of %s", r.ReasoningEffort, strings.Join(reasoningEfforts, " | "))
	}
	// NaN and the infinities are rejected alongside a negative, because they
	// fail LATER and worse: yaml accepts .nan and .inf, validation would pass
	// them, and then every request fails at encode time — json cannot represent
	// a non-finite number — so a config that booted would break each call.
	if math.IsNaN(r.PreferredMaxLatencyP90) || math.IsInf(r.PreferredMaxLatencyP90, 0) {
		return fmt.Errorf("ai: routing config: preferred_max_latency_p90 must be a finite number of seconds, got %g", r.PreferredMaxLatencyP90)
	}
	if r.PreferredMaxLatencyP90 < 0 {
		return fmt.Errorf("ai: routing config: preferred_max_latency_p90 %g is negative", r.PreferredMaxLatencyP90)
	}
	return nil
}

// --- routing-config integration ------------------------------------------
//
// These three live beside the preferences rather than in routing.go because they
// are about THIS type: what a binding inherits when it declares nothing, which
// bindings the preferences can reach at all, and what the parser refuses.
// routing.go calls them. Keeping them here means a change to the field set and
// a change to the rules governing it are one file rather than two.

// applyUpstreamDefaults gives every broker binding that declared no upstream
// preferences the product default, which is reliability over price.
//
// Only a binding whose base_url names the broker: these are its parameters, and
// a direct vendor on the same OpenAI wire would be sent fields it never asked
// for. Only an ABSENT declaration, never an empty one — `routing: {}` is an
// operator saying "the broker's own routing, please", and overwriting that with
// a default would make the opt-out unwritable.
//
// The embeddings lane is left alone deliberately. Its calls are one forward
// pass each, so the tail this default exists to remove is not a thing that
// happens there, and no embedding model is served at fp4-versus-bf16 stakes.
func (cfg *RoutingConfig) applyUpstreamDefaults() {
	for tier, binding := range cfg.Tiers {
		if binding.Routing != nil || !upstreamPreferencesApply(binding) {
			continue
		}
		binding.Routing = DefaultOpenRouterRouting()
		cfg.Tiers[tier] = binding
	}
}

// upstreamPreferencesApply reports whether a binding is the broker case that
// upstream-selection preferences describe.
func upstreamPreferencesApply(binding ProviderConfig) bool {
	return binding.Provider == providerOpenAICompatible && IsOpenRouterHost(binding.BaseURL)
}

// validateUpstreamPreferences refuses a `routing:` block on a binding that
// cannot honour it, and refuses a value the broker would silently ignore.
//
// Refused rather than dropped, because dropping is what the broker itself does
// with an unknown preference: a deployment would then run price-weighted while
// its config file said `sort: throughput`, and the only symptom would be the
// latency the setting was written to remove. An operator who wrote the block
// gets told which of the two reasons it cannot apply — the provider or the host
// — because those are different edits.
func validateUpstreamPreferences(tier string, binding ProviderConfig) error {
	if binding.Routing == nil {
		return nil
	}
	if binding.Provider != providerOpenAICompatible {
		return fmt.Errorf("ai: routing config: tier %s: `routing` is upstream selection for a broker and provider %s serves one model from one host; remove the block",
			tier, binding.Provider)
	}
	if !IsOpenRouterHost(binding.BaseURL) {
		return fmt.Errorf("ai: routing config: tier %s: `routing` names OpenRouter's own upstream-selection fields and base_url %q is not an OpenRouter host; remove the block, or point the binding at the broker",
			tier, binding.BaseURL)
	}
	if err := binding.Routing.Validate(); err != nil {
		return fmt.Errorf("%w (tier %s)", err, tier)
	}
	return nil
}

// refuseEmptyOrRepeated holds a preference list to the shape the schema
// declares: written means non-empty, and each entry said once.
//
// A written-but-empty list is the mistake worth naming rather than ignoring —
// `only: []` reads as "no host may serve this" and is treated by the broker as
// no preference at all, which is the opposite. A repeat is harmless to the
// broker and means the operator believes something about the second one.
func refuseEmptyOrRepeated(field string, list []string) error {
	if list == nil {
		return nil
	}
	if len(list) == 0 {
		return fmt.Errorf("ai: routing config: `%s` is written with no entries; omit the field to state no preference", field)
	}
	seen := make(map[string]bool, len(list))
	for _, entry := range list {
		if seen[entry] {
			return fmt.Errorf("ai: routing config: `%s` names %q twice", field, entry)
		}
		seen[entry] = true
	}
	return nil
}
