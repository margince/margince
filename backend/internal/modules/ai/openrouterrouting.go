// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"reflect"
	"slices"
	"strings"
)

// OpenRouterRouting is a binding's request to the OpenRouter broker: which
// hosts may serve it and how (Provider), and how hard the model thinks
// (Reasoning).
//
// A broker fronts many inference hosts per model and its own default weights
// them by price, so one model id arrives at fp4 on one call and bf16 on the
// next, with latency to match. These preferences are how a deployment says
// which trade it wants.
//
// The JSON spelling IS OpenRouter's request body: the stored value, the served
// schema and the wire share one name per field, so a preference cannot mean a
// different thing depending on which door it came through. The flat spelling
// this type had before (`sort`, `reasoning_effort` … at top level) is still
// read at every door; openrouterlegacy.go maps it.
type OpenRouterRouting struct {
	Provider  OpenRouterProvider   `yaml:"provider" json:"provider,omitzero"`
	Reasoning *OpenRouterReasoning `yaml:"reasoning,omitempty" json:"reasoning,omitempty"`
}

// OpenRouterProvider is the broker's `provider` object. Every field is
// omitempty: a broker reads an explicitly-null preference as a preference. The
// pointers are there because false is a real choice that must not read as unset.
//
// Only, Ignore, AllowFallbacks, ZDR, DataCollection and EnforceDistillableText
// say WHICH hosts may read the request; they live on the connection
// (connectionKeys). The rest say how one model is served and live on a tier.
type OpenRouterProvider struct {
	// The fields the flat wire carried come first, in its order, so the bytes a
	// pre-existing config sends are unchanged.
	Only                   []string          `yaml:"only,omitempty" json:"only,omitempty"`
	Ignore                 []string          `yaml:"ignore,omitempty" json:"ignore,omitempty"`
	Quantizations          []string          `yaml:"quantizations,omitempty" json:"quantizations,omitempty"`
	Sort                   *OpenRouterSort   `yaml:"sort,omitempty" json:"sort,omitempty"`
	RequireParameters      *bool             `yaml:"require_parameters,omitempty" json:"require_parameters,omitempty"`
	AllowFallbacks         *bool             `yaml:"allow_fallbacks,omitempty" json:"allow_fallbacks,omitempty"`
	PreferredMaxLatency    *OpenRouterPctile `yaml:"preferred_max_latency,omitempty" json:"preferred_max_latency,omitempty"`
	Order                  []string          `yaml:"order,omitempty" json:"order,omitempty"`
	DataCollection         string            `yaml:"data_collection,omitempty" json:"data_collection,omitempty"`
	ZDR                    *bool             `yaml:"zdr,omitempty" json:"zdr,omitempty"`
	EnforceDistillableText *bool             `yaml:"enforce_distillable_text,omitempty" json:"enforce_distillable_text,omitempty"`
	MaxPrice               *OpenRouterPrice  `yaml:"max_price,omitempty" json:"max_price,omitempty"`
	PreferredMinThroughput *OpenRouterPctile `yaml:"preferred_min_throughput,omitempty" json:"preferred_min_throughput,omitempty"`
}

// OpenRouterSort orders candidates by price, throughput or latency. It reorders
// rather than filters, and setting it disables the broker's load balancing.
// Partition "none" sorts across every model of a fallback list at once.
type OpenRouterSort struct {
	By        string `json:"by"`
	Partition string `json:"partition,omitempty"`
	// asObject records the {by} spelling, so a refusal points at `sort.by`
	// where the operator wrote it rather than at `sort`.
	asObject bool
}

// OpenRouterPrice is the most a request may cost, in USD per million tokens
// (prompt, completion) or per request or image. Hosts above it are filtered out.
type OpenRouterPrice struct {
	Prompt     *float64 `json:"prompt,omitempty"`
	Completion *float64 `json:"completion,omitempty"`
	Request    *float64 `json:"request,omitempty"`
	Image      *float64 `json:"image,omitempty"`
}

// OpenRouterPctile is a soft threshold: one number for every percentile, or a
// value per percentile. A host past it is moved down the list, never removed.
type OpenRouterPctile struct {
	All *float64 `json:"-"`
	P50 *float64 `json:"p50,omitempty"`
	P75 *float64 `json:"p75,omitempty"`
	P90 *float64 `json:"p90,omitempty"`
	P99 *float64 `json:"p99,omitempty"`
}

// OpenRouterReasoning is the reasoning-model control block. Unset leaves each
// host its own default, which varies cost, latency and — when thinking exhausts
// the output budget first — whether there is an answer at all.
type OpenRouterReasoning struct {
	Effort    string `yaml:"effort,omitempty" json:"effort,omitempty"`
	MaxTokens *int   `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	Exclude   *bool  `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Enabled   *bool  `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// MarshalJSON writes a sort with no partition as the bare string, which is the
// spelling every existing request used, so the wire for an unchanged config is
// byte-identical.
func (s OpenRouterSort) MarshalJSON() ([]byte, error) {
	if s.Partition == "" {
		return json.Marshal(s.By)
	}
	return json.Marshal(struct {
		By        string `json:"by"`
		Partition string `json:"partition"`
	}{s.By, s.Partition})
}

// MarshalJSON writes a single threshold as the bare number OpenRouter accepts
// for every percentile at once.
func (p OpenRouterPctile) MarshalJSON() ([]byte, error) {
	if p.All != nil {
		return json.Marshal(*p.All)
	}
	type percentiles OpenRouterPctile
	return json.Marshal(percentiles(p))
}

func (p OpenRouterProvider) isEmpty() bool {
	return reflect.ValueOf(p).IsZero()
}

func (r *OpenRouterReasoning) isEmpty() bool {
	return r == nil || *r == OpenRouterReasoning{}
}

// IsEmpty reports whether an operator wrote a declaration that asks for
// nothing — `routing: {}`, the explicit opt-out that takes the broker's own
// price-weighted routing. `{"provider":{}}` is the same opt-out.
func (r *OpenRouterRouting) IsEmpty() bool {
	return r == nil || (r.Provider.isEmpty() && r.Reasoning.isEmpty())
}

// providerWire renders the `provider` object, or nil when these preferences
// would say nothing: an object of defaults would disable load balancing by
// accident.
func (r *OpenRouterRouting) providerWire() *OpenRouterProvider {
	if r == nil || r.Provider.isEmpty() {
		return nil
	}
	return &r.Provider
}

// reasoningWire renders the `reasoning` block, or nil when none is set —
// leaving the host's own default in place.
func (r *OpenRouterRouting) reasoningWire() *OpenRouterReasoning {
	if r == nil || r.Reasoning.isEmpty() {
		return nil
	}
	return r.Reasoning
}

// reasoningEffort is the effort this value pins, "" for none.
func (r *OpenRouterRouting) reasoningEffort() string {
	if r == nil || r.Reasoning == nil {
		return ""
	}
	return r.Reasoning.Effort
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
	return &OpenRouterRouting{Provider: OpenRouterProvider{
		Sort:              &OpenRouterSort{By: SortThroughput},
		Quantizations:     []string{"fp16", "bf16"},
		RequireParameters: &requireParameters,
	}}
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
	reasoningEfforts = []string{effortMax, effortXHigh, effortHigh, effortMedium, effortLow, effortMinimal, effortNone}
)

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
		binding.Routing = UpstreamPreferencesFor(binding)
		cfg.Tiers[tier] = binding
	}
}

// UpstreamPreferencesFor is the upstream preferences a binding is served under:
// its own declaration, the product default for an undeclared broker binding, and
// nil for a binding no preference reaches. Exported so the certification lane
// records what this rule applied rather than a copy of it.
func UpstreamPreferencesFor(binding ProviderConfig) *OpenRouterRouting {
	if binding.Routing != nil || !UpstreamPreferencesApply(binding) {
		return binding.Routing
	}
	return DefaultOpenRouterRouting()
}

// UpstreamPreferencesApply reports whether a binding is the broker case that
// upstream-selection preferences describe.
func UpstreamPreferencesApply(binding ProviderConfig) bool {
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
func validateUpstreamPreferences(path string, binding ProviderConfig) error {
	if binding.Routing == nil {
		return nil
	}
	if binding.Provider != providerOpenAICompatible {
		return invalidAt(path, fmt.Sprintf("is upstream selection for a broker and provider %s serves one model from one host; remove the block",
			binding.Provider))
	}
	if !IsOpenRouterHost(binding.BaseURL) {
		return invalidAt(path, fmt.Sprintf("names OpenRouter's own upstream-selection fields and base_url %q is not an OpenRouter host; remove the block, or point the binding at the broker",
			binding.BaseURL))
	}
	return binding.Routing.Validate(path)
}

// TierRoutingPath and EmbeddingsRoutingPath address a lane's routing in the
// routing document, the root every fault path below them hangs from.
func TierRoutingPath(tier Tier) string { return "tiers." + string(tier) + ".routing" }

const EmbeddingsRoutingPath = "embeddings.routing"

// validateEmbeddingsRouting admits on the embeddings lane only the connection
// keys — which hosts may read the text, and under what privacy. The lane embeds
// the same text the chat tiers send, so a residency pin or a retention rule the
// chat tiers carry and the embeddings lane could not would leave the one lane
// that sees every document free to leave it. The rest bound a completion's tail
// or its thinking, and an embedding is one forward pass with neither.
func validateEmbeddingsRouting(binding ProviderConfig) error {
	r := binding.Routing
	if r == nil {
		return nil
	}
	if err := validateUpstreamPreferences(EmbeddingsRoutingPath, binding); err != nil {
		return err
	}
	// An allowlist, not a list of what is refused: a preference added to
	// OpenRouterRouting later is refused here until somebody decides it belongs,
	// which is what the generated schema's additionalProperties:false says too.
	if rest := r.withoutPins(); !rest.IsEmpty() {
		return refuseOnEmbeddings(rest)
	}
	return nil
}

// refuseOnEmbeddings names each key rest carries by its path: the lane takes
// only the connection's host rules, which say which hosts may read the text,
// and every other preference bounds a completion.
func refuseOnEmbeddings(rest *OpenRouterRouting) error {
	why := "is not taken on the embeddings lane, which accepts only the connection's host rules (" +
		strings.Join(connectionKeys, ", ") + "). Remove it."
	raw, err := rest.RequestJSON()
	var written map[string]map[string]json.RawMessage
	if err == nil {
		err = json.Unmarshal(raw, &written)
	}
	if err != nil {
		return fmt.Errorf("ai: reading the embeddings lane's routing: %w", err)
	}
	var errs []error
	for _, block := range slices.Sorted(maps.Keys(written)) {
		for _, key := range slices.Sorted(maps.Keys(written[block])) {
			errs = append(errs, invalidAt(joinPath(EmbeddingsRoutingPath, block+"."+key), why))
		}
	}
	return joinFaults(errs...)
}

// refuseEmptyOrRepeated holds a preference list to the shape the schema
// declares: written means non-empty, and each entry said once.
//
// A written-but-empty list is the mistake worth naming rather than ignoring —
// `only: []` reads as "no host may serve this" and is treated by the broker as
// no preference at all, which is the opposite. A repeat is harmless to the
// broker and means the operator believes something about the second one.
func refuseEmptyOrRepeated(path string, list []string) error {
	if list == nil {
		return nil
	}
	if len(list) == 0 {
		return invalidAt(path, "is written with no entries; omit the field to state no preference")
	}
	seen := make(map[string]bool, len(list))
	for i, entry := range list {
		if seen[entry] {
			return invalidAt(fmt.Sprintf("%s[%d]", path, i), fmt.Sprintf("names %q twice", entry))
		}
		seen[entry] = true
	}
	return nil
}
