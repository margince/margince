// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
)

// ProviderSettings is what one provider is configured with, independent of any
// lane: the host it is dialled at and, for a broker, where its requests may be
// served.
//
// The lanes' BaseURL and the pins inside their Routing are resolved from here by
// finalize and are never the authority; canonical() strips them before a
// document is stored.
type ProviderSettings struct {
	BaseURL string `yaml:"base_url" json:"base_url,omitempty"`
	// Upstream holds only the pins — only, ignore, allow_fallbacks — because
	// residency is the provider's. How a model is served (sort, quantizations,
	// thinking) stays on each tier: two models behind one broker need different
	// answers.
	Upstream *OpenRouterRouting `yaml:"upstream" json:"upstream,omitempty"`
	// Location is the Vertex AI location a gemini_vertex provider is served
	// from, which is where Google processes the call for every lane on it.
	Location string `yaml:"location" json:"location,omitempty"`
}

// The lane labels a lift warning and BoundProviders name, in the words every
// routing refusal already uses for the two single lanes.
const (
	embeddingsLaneLabel = "the embeddings lane"
	decisionsLaneLabel  = "the decisions lane"
	providerEntryLabel  = "the provider entry"
)

func tierLabel(tier Tier) string { return "tier " + string(tier) }

// sortedTiers is the deterministic lane order a lift resolves disagreements in.
func (cfg RoutingConfig) sortedTiers() []Tier {
	return slices.Sorted(maps.Keys(cfg.Tiers))
}

// connectionKeys are the `provider` keys that say WHICH hosts may read a
// request. They live on the connection, so every tier on it is served under
// the same privacy and no tier can loosen it.
// The connection keys and the host lists as OpenRouter spells them, and two words the decoder
// reads by: the provider block's name and JSON's null.
const (
	keyOnly               = "only"
	keyIgnore             = "ignore"
	keyAllowFallbacks     = "allow_fallbacks"
	keyZDR                = "zdr"
	keyDataCollection     = "data_collection"
	keyEnforceDistillable = "enforce_distillable_text"
	keyOrder              = "order"
	keyQuantizations      = "quantizations"
	keyRequireParameters  = "require_parameters"
	keySort               = "sort"
	blockProvider         = "provider"
	jsonNull              = "null"
)

var connectionKeys = []string{keyOnly, keyIgnore, keyAllowFallbacks, keyZDR, keyDataCollection, keyEnforceDistillable}

// clone copies the preferences so a lane never aliases its provider's block or a
// sibling lane's.
func (r *OpenRouterRouting) clone() *OpenRouterRouting {
	if r == nil {
		return nil
	}
	out := *r
	p := &out.Provider
	p.Order, p.Only, p.Ignore, p.Quantizations = slices.Clone(p.Order), slices.Clone(p.Only), slices.Clone(p.Ignore), slices.Clone(p.Quantizations)
	p.AllowFallbacks, p.RequireParameters = clonePtr(p.AllowFallbacks), clonePtr(p.RequireParameters)
	p.ZDR, p.EnforceDistillableText = clonePtr(p.ZDR), clonePtr(p.EnforceDistillableText)
	p.Sort, p.MaxPrice = clonePtr(p.Sort), clonePtr(p.MaxPrice)
	p.PreferredMinThroughput, p.PreferredMaxLatency = clonePtr(p.PreferredMinThroughput), clonePtr(p.PreferredMaxLatency)
	out.Reasoning = clonePtr(r.Reasoning)
	return &out
}

// clonePtr copies the value behind a pointer. The structs it copies hold only
// pointers to numbers, which a lane never writes through, so one level is deep
// enough.
func clonePtr[T any](v *T) *T {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

// pins is the part of r that says WHICH hosts may serve the request — its
// connection keys — or nil when r writes none. Written-but-empty lists count as
// written, so the validator still refuses an `only: []`.
func (r *OpenRouterRouting) pins() *OpenRouterRouting {
	if r == nil {
		return nil
	}
	p := r.Provider
	conn := OpenRouterProvider{
		Only: p.Only, Ignore: p.Ignore, AllowFallbacks: p.AllowFallbacks,
		ZDR: p.ZDR, DataCollection: p.DataCollection, EnforceDistillableText: p.EnforceDistillableText,
	}
	if p.Only == nil && p.Ignore == nil && conn.isEmpty() {
		return nil
	}
	return (&OpenRouterRouting{Provider: conn}).clone()
}

// withoutPins is r with its connection keys cleared. A block that held only
// them becomes `{}`, not nil, so a lane that opted out of the product default
// stays opted out.
func (r *OpenRouterRouting) withoutPins() *OpenRouterRouting {
	out := r.clone()
	if out != nil {
		out.Provider.setConnection(OpenRouterProvider{})
	}
	return out
}

// withPins is a fresh copy of r carrying pins' connection keys. With no pins,
// r is copied as is, so an absent block stays absent.
func (r *OpenRouterRouting) withPins(pins *OpenRouterRouting) *OpenRouterRouting {
	out := r.clone()
	if pins == nil {
		return out
	}
	if out == nil {
		out = &OpenRouterRouting{}
	}
	out.Provider.setConnection(pins.clone().Provider)
	return out
}

// setConnection replaces p's connection keys with conn's.
func (p *OpenRouterProvider) setConnection(conn OpenRouterProvider) {
	p.Only, p.Ignore, p.AllowFallbacks = conn.Only, conn.Ignore, conn.AllowFallbacks
	p.ZDR, p.DataCollection, p.EnforceDistillableText = conn.ZDR, conn.DataCollection, conn.EnforceDistillableText
}

// resolveProviders fills every lane's host and pins from its provider. A tier's
// pins land on top of its product default when it declared no routing, so an
// EU-pinned broker tier keeps the default sort and precision; a tier that wrote
// `{}` gets the pins alone. The embeddings lane keeps its own host and pins when
// it names them, and otherwise reads its provider's. Each lane gets its own copy.
func (cfg RoutingConfig) resolveProviders() RoutingConfig {
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for tier, lane := range cfg.Tiers {
		// An unknown provider's lane keeps its own host, as the lift left it.
		if !knownProvider(lane.Provider) {
			tiers[tier] = lane
			continue
		}
		settings := cfg.Providers[lane.Provider]
		lane.BaseURL, lane.Location = settings.BaseURL, settings.Location
		lane.Routing = UpstreamPreferencesFor(lane).withPins(settings.Upstream.pins())
		tiers[tier] = lane
	}
	cfg.Tiers = tiers
	if knownProvider(cfg.Embeddings.Provider) {
		cfg.Embeddings.ProviderConfig = cfg.Embeddings.resolved(cfg.Providers[cfg.Embeddings.Provider])
	}
	if cfg.Decisions != nil && knownProvider(cfg.Decisions.Provider) {
		decisions := *cfg.Decisions
		decisions.BaseURL = cfg.Providers[decisions.Provider].BaseURL
		cfg.Decisions = &decisions
	}
	return cfg
}

// resolved is the embeddings lane as served. Its own base_url is a separate
// embeddings server (one model per vLLM process), and its provider's pins name
// hosts behind the provider's broker, so they reach the lane only when it is
// served there and states none of its own.
func (e EmbeddingsConfig) resolved(settings ProviderSettings) ProviderConfig {
	lane := e.ProviderConfig
	lane.Routing = lane.Routing.clone()
	if lane.Location == "" {
		lane.Location = settings.Location
	}
	if lane.BaseURL != "" {
		return lane
	}
	lane.BaseURL = settings.BaseURL
	if lane.Routing.pins() == nil {
		lane.Routing = lane.Routing.withPins(settings.Upstream.pins())
	}
	return lane
}

// canonical is the document as stored: providers hold every host and pin they
// can, tiers keep only their serving preferences, and the embeddings lane only
// what overrides its provider. It lifts first, so a row stored in the old
// per-lane shape and its lifted twin are one document — the ETag (Revision)
// must see a host whichever shape it is read in.
func (cfg RoutingConfig) canonical() RoutingConfig {
	return cfg.liftLaneProviderFields(slog.New(slog.DiscardHandler))
}

// validateProviderEntries holds each provider entry to its shape. An entry no
// lane binds gets only these checks, so an admin can set a host before binding
// it; a bound host is left to the lane rules, which name the lane and the
// endpoint rule it breaks once resolved.
func (cfg RoutingConfig) validateProviderEntries() error {
	// A lane checks the host it dials; an embeddings server of its own dials
	// its own, so the provider host it shadows is checked here, since a key
	// test and a model list still dial it.
	reads := cfg.BoundProviders()
	if cfg.Embeddings.BaseURL != "" {
		reads[cfg.Embeddings.Provider] = slices.DeleteFunc(reads[cfg.Embeddings.Provider], func(label string) bool { return label == embeddingsLaneLabel })
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Providers)) {
		if err := validateProviderEntry(name, cfg.Providers[name], len(reads[name]) > 0); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderEntry(name string, settings ProviderSettings, bound bool) error {
	if !knownProvider(name) {
		return fmt.Errorf("ai: routing config: providers: %q is not a provider this build knows (have: %s)",
			name, strings.Join(providerNames(), ", "))
	}
	if err := validateUnboundHost(name, settings.BaseURL, bound); err != nil {
		return err
	}
	if settings.Location != "" && name != providerGeminiVertex {
		return fmt.Errorf("ai: routing config: providers: %s: `location` names a Vertex AI location and only gemini_vertex is served from one; remove it", name)
	}
	if settings.Location != "" && !vertexLocationShape.MatchString(settings.Location) {
		return fmt.Errorf("ai: routing config: providers: %s: location must be eu, us, global, or a region such as europe-west4", name)
	}
	upstream := settings.Upstream
	if upstream == nil {
		return nil
	}
	if name != providerOpenAICompatible {
		return fmt.Errorf("ai: routing config: providers: %s: `upstream` is upstream selection for a broker and %s serves one model from one host; remove the block",
			name, name)
	}
	if settings.BaseURL != "" && !IsOpenRouterHost(settings.BaseURL) {
		return fmt.Errorf("ai: routing config: providers: %s: `upstream` names OpenRouter's own upstream-selection fields and base_url is not an OpenRouter host; remove the block, or point the provider at the broker", name)
	}
	path := "providers." + name + ".upstream"
	if !upstream.withoutPins().IsEmpty() {
		return invalidAt(path, "takes only "+strings.Join(connectionKeys, ", ")+" — which hosts may read a request; "+
			"how a model is served (sort, quantizations, max_price, reasoning …) is set per tier, under its `routing`")
	}
	return upstream.Validate(path)
}

// validateUnboundHost holds the host of an entry no lane binds. The key test
// and the model list dial it too, so it meets the egress rule here rather than
// at the Test button; a bound one meets it on each lane, whose refusal names
// the lane. The URL is not echoed: it may carry userinfo.
func validateUnboundHost(name, baseURL string, bound bool) error {
	if bound || baseURL == "" {
		return nil
	}
	if !isFetchableURL(baseURL) {
		return fmt.Errorf("ai: routing config: providers: %s: base_url is not an http(s) URL with a host; give the vendor host root, e.g. https://openrouter.ai/api", name)
	}
	return requireDialableEndpoint("providers: "+name, name, baseURL)
}

// BoundProviders reports, for each provider some lane binds, the lanes binding
// it in lift order — what a provider-settings change re-resolves.
func (cfg RoutingConfig) BoundProviders() map[string][]string {
	bound := map[string][]string{}
	add := func(provider, label string) {
		if provider != "" {
			bound[provider] = append(bound[provider], label)
		}
	}
	for _, tier := range cfg.sortedTiers() {
		add(cfg.Tiers[tier].Provider, tierLabel(tier))
	}
	add(cfg.Embeddings.Provider, embeddingsLaneLabel)
	if cfg.Decisions != nil {
		add(cfg.Decisions.Provider, decisionsLaneLabel)
	}
	return bound
}
