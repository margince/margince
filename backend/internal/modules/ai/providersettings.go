// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"log/slog"
	"maps"
	"reflect"
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

// clone copies the preferences so a lane never aliases its provider's block or a
// sibling lane's.
func (r *OpenRouterRouting) clone() *OpenRouterRouting {
	if r == nil {
		return nil
	}
	out := *r
	out.Only, out.Ignore, out.Quantizations = slices.Clone(r.Only), slices.Clone(r.Ignore), slices.Clone(r.Quantizations)
	if r.RequireParameters != nil {
		v := *r.RequireParameters
		out.RequireParameters = &v
	}
	if r.AllowFallbacks != nil {
		v := *r.AllowFallbacks
		out.AllowFallbacks = &v
	}
	return &out
}

// pins is the part of r that says WHICH hosts may serve the request, or nil
// when r writes none. Written-but-empty lists count as written, so the
// validator still refuses an `only: []`.
func (r *OpenRouterRouting) pins() *OpenRouterRouting {
	if r == nil || (r.Only == nil && r.Ignore == nil && r.AllowFallbacks == nil) {
		return nil
	}
	return (&OpenRouterRouting{Only: r.Only, Ignore: r.Ignore, AllowFallbacks: r.AllowFallbacks}).clone()
}

// withoutPins is r with its pins cleared. A block that held only pins becomes
// `{}`, not nil, so a lane that opted out of the product default stays opted out.
func (r *OpenRouterRouting) withoutPins() *OpenRouterRouting {
	out := r.clone()
	if out != nil {
		out.Only, out.Ignore, out.AllowFallbacks = nil, nil, nil
	}
	return out
}

// withPins is a fresh copy of r carrying pins. With no pins, r is copied as is,
// so an absent block stays absent.
func (r *OpenRouterRouting) withPins(pins *OpenRouterRouting) *OpenRouterRouting {
	out := r.clone()
	if pins == nil {
		return out
	}
	if out == nil {
		out = &OpenRouterRouting{}
	}
	pinned := pins.clone()
	out.Only, out.Ignore, out.AllowFallbacks = pinned.Only, pinned.Ignore, pinned.AllowFallbacks
	return out
}

// providerLift carries a lift's state across lanes: the providers being built
// and which lane each kept value came from, for the warning that names both.
type providerLift struct {
	providers map[string]ProviderSettings
	hostFrom  map[string]string
	pinsFrom  map[string]string
	log       *slog.Logger
}

// liftLaneProviderFields moves the host and pins written on lanes — the shape
// every routing document had before providers held them — onto the provider,
// leaving each lane's serving preferences where they are. Lanes are visited
// tiers by name, then embeddings, then decisions; the first value wins and each
// disagreeing lane is one warning. A lane value equal to its provider's is
// cleared silently, so the lift is idempotent and safe over a resolved config.
func (cfg RoutingConfig) liftLaneProviderFields(log *slog.Logger) RoutingConfig {
	lift := providerLift{providers: maps.Clone(cfg.Providers), hostFrom: map[string]string{}, pinsFrom: map[string]string{}, log: log}
	if lift.providers == nil {
		lift.providers = map[string]ProviderSettings{}
	}
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for _, tier := range cfg.sortedTiers() {
		tiers[tier] = lift.lane(tierLabel(tier), cfg.Tiers[tier])
	}
	cfg.Tiers = tiers
	cfg.Embeddings.ProviderConfig = lift.lane(embeddingsLaneLabel, cfg.Embeddings.ProviderConfig)
	if cfg.Decisions != nil {
		decisions := *cfg.Decisions
		lift.host(decisionsLaneLabel, decisions.Provider, decisions.BaseURL)
		decisions.BaseURL = ""
		cfg.Decisions = &decisions
	}
	if len(lift.providers) == 0 {
		lift.providers = nil
	}
	cfg.Providers = lift.providers
	return cfg
}

// lane lifts one chat lane's host and pins and returns it without them.
func (l providerLift) lane(label string, lane ProviderConfig) ProviderConfig {
	l.host(label, lane.Provider, lane.BaseURL)
	if pins := lane.Routing.pins(); pins != nil {
		l.pins(label, lane.Provider, pins)
	}
	lane.BaseURL, lane.Routing = "", lane.Routing.withoutPins()
	return lane
}

func (l providerLift) host(label, provider, baseURL string) {
	if baseURL == "" {
		return
	}
	settings := l.providers[provider]
	switch {
	case settings.BaseURL == "":
		settings.BaseURL = baseURL
		l.providers[provider] = settings
		l.hostFrom[provider] = label
	case !sameEndpoint(settings.BaseURL, baseURL):
		l.warn("ai: routing: lane host differs from its provider's; the provider's wins", provider, l.hostFrom, label)
	}
}

func (l providerLift) pins(label, provider string, pins *OpenRouterRouting) {
	settings := l.providers[provider]
	switch {
	case settings.Upstream == nil:
		settings.Upstream = pins
		l.providers[provider] = settings
		l.pinsFrom[provider] = label
	case !reflect.DeepEqual(settings.Upstream.pins(), pins):
		l.warn("ai: routing: lane upstream pins differ from its provider's; the provider's win", provider, l.pinsFrom, label)
	}
}

func (l providerLift) warn(msg, provider string, from map[string]string, dropped string) {
	kept := from[provider]
	if kept == "" {
		kept = providerEntryLabel
	}
	l.log.Warn(msg, "provider", provider, "kept_from", kept, "dropped", dropped)
}

// resolveProviders fills every lane's host and pins from its provider. A tier's
// pins land on top of its product default when it declared no routing, so an
// EU-pinned broker tier keeps the default sort and precision; a tier that wrote
// `{}` gets the pins alone. Each lane gets its own copy.
func (cfg RoutingConfig) resolveProviders() RoutingConfig {
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for tier, lane := range cfg.Tiers {
		settings := cfg.Providers[lane.Provider]
		lane.BaseURL = settings.BaseURL
		lane.Routing = UpstreamPreferencesFor(lane).withPins(settings.Upstream.pins())
		tiers[tier] = lane
	}
	cfg.Tiers = tiers
	settings := cfg.Providers[cfg.Embeddings.Provider]
	cfg.Embeddings.BaseURL = settings.BaseURL
	cfg.Embeddings.Routing = cfg.Embeddings.Routing.withPins(settings.Upstream.pins())
	if cfg.Decisions != nil {
		decisions := *cfg.Decisions
		decisions.BaseURL = cfg.Providers[decisions.Provider].BaseURL
		cfg.Decisions = &decisions
	}
	return cfg
}

// canonical is the document as stored: providers hold every host and pin, and
// lanes keep only their own serving preferences.
func (cfg RoutingConfig) canonical() RoutingConfig {
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for tier, lane := range cfg.Tiers {
		lane.BaseURL, lane.Routing = "", lane.Routing.withoutPins()
		tiers[tier] = lane
	}
	if cfg.Tiers != nil {
		cfg.Tiers = tiers
	}
	cfg.Embeddings.BaseURL, cfg.Embeddings.Routing = "", cfg.Embeddings.Routing.withoutPins()
	if cfg.Decisions != nil {
		decisions := *cfg.Decisions
		decisions.BaseURL = ""
		cfg.Decisions = &decisions
	}
	return cfg
}

// validateProviderEntries holds each provider entry to its shape. An entry no
// lane binds gets only these checks, so an admin can set a host before binding
// it; a bound host is left to the lane rules, which name the lane and the
// endpoint rule it breaks once resolved.
func (cfg RoutingConfig) validateProviderEntries() error {
	bound := cfg.BoundProviders()
	for _, name := range slices.Sorted(maps.Keys(cfg.Providers)) {
		if err := validateProviderEntry(name, cfg.Providers[name], len(bound[name]) > 0); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderEntry(name string, settings ProviderSettings, bound bool) error {
	if _, known := providerByName(name); !known {
		return fmt.Errorf("ai: routing config: providers: %q is not a provider this build knows (have: %s)",
			name, strings.Join(providerNames(), ", "))
	}
	// The URL is not echoed: it may carry userinfo, and the lane rule a bound
	// host meets refuses that without repeating it either.
	if !bound && settings.BaseURL != "" && !isFetchableURL(settings.BaseURL) {
		return fmt.Errorf("ai: routing config: providers: %s: base_url is not an http(s) URL with a host; give the vendor host root, e.g. https://openrouter.ai/api", name)
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
	if !upstream.withoutPins().IsEmpty() {
		return fmt.Errorf("ai: routing config: providers: %s: `upstream` takes only `only`, `ignore` and `allow_fallbacks` — where requests are served; "+
			"sort, quantizations, require_parameters, preferred_max_latency_p90 and reasoning_effort are set per tier, under its `routing`", name)
	}
	if err := upstream.Validate(); err != nil {
		return fmt.Errorf("%w (providers: %s)", err, name)
	}
	return nil
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
