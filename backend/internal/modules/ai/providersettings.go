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
// lane: the host it is dialled at and, for a broker, the upstream-selection
// preferences every lane on it is served under.
//
// The lanes' own BaseURL and Routing are resolved from here by finalize and are
// never the authority; canonical() strips them before a document is stored.
type ProviderSettings struct {
	BaseURL string `yaml:"base_url" json:"base_url,omitempty"`
	// Upstream keeps ProviderConfig.Routing's three states: absent takes the
	// product default on a chat tier, `{}` asks for no preferences. Its
	// ReasoningEffort is refused, because how hard a model thinks is the tier's.
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
// sibling lane's: one lane's merged effort must not reach the next.
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

// projectEmbeddings keeps the preferences that say WHICH hosts may read the
// text. The embeddings lane reads every document the chat tiers do, so a
// residency pin must reach it; the rest bound a completion's tail or thinking,
// and an embedding is one forward pass with neither.
//
// A block that pins nothing projects to nil rather than `{}`: on this lane the
// two send the same request, and nil is what the lane stored before its
// preferences moved onto the provider, so the routing version stays put.
func projectEmbeddings(upstream *OpenRouterRouting) *OpenRouterRouting {
	if upstream == nil {
		return nil
	}
	kept := &OpenRouterRouting{Only: upstream.Only, Ignore: upstream.Ignore, AllowFallbacks: upstream.AllowFallbacks}
	if kept.IsEmpty() && !upstream.IsEmpty() {
		return nil
	}
	return kept.clone()
}

// providerLift carries a lift's state across lanes: the providers being built
// and which lane each kept value came from, for the warning that names both.
type providerLift struct {
	providers    map[string]ProviderSettings
	hostFrom     map[string]string
	upstreamFrom map[string]string
	tierBound    map[string]bool
	log          *slog.Logger
}

// liftLaneProviderFields moves the host and upstream preferences written on
// lanes — the shape every routing document had before providers held them —
// onto the provider. Lanes are visited tiers by name, then embeddings, then
// decisions; the first value wins and each disagreeing lane is one warning.
//
// A lane value equal to what its provider would resolve to is cleared silently,
// which makes the lift idempotent and safe over an already-resolved config.
func (cfg RoutingConfig) liftLaneProviderFields(log *slog.Logger) RoutingConfig {
	lift := providerLift{
		providers: maps.Clone(cfg.Providers), hostFrom: map[string]string{}, upstreamFrom: map[string]string{},
		tierBound: map[string]bool{}, log: log,
	}
	if lift.providers == nil {
		lift.providers = map[string]ProviderSettings{}
	}
	for _, lane := range cfg.Tiers {
		lift.tierBound[lane.Provider] = true
	}
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for _, tier := range cfg.sortedTiers() {
		tiers[tier] = lift.lane(tierLabel(tier), cfg.Tiers[tier], false)
	}
	cfg.Tiers = tiers
	cfg.Embeddings.ProviderConfig = lift.lane(embeddingsLaneLabel, cfg.Embeddings.ProviderConfig, true)
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

// lane lifts one chat lane's host and preferences and returns it cleared.
func (l providerLift) lane(label string, lane ProviderConfig, embeddings bool) ProviderConfig {
	l.host(label, lane.Provider, lane.BaseURL)
	if routing := lane.Routing.clone(); routing != nil {
		if lane.ReasoningEffort == "" {
			lane.ReasoningEffort = routing.ReasoningEffort
		}
		routing.ReasoningEffort = ""
		l.upstream(label, lane, routing, embeddings)
	}
	lane.BaseURL, lane.Routing = "", nil
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

// upstream lifts a lane's effort-free preferences. The embeddings lane may seed
// a provider only no tier binds: a tier's block is the fuller one, and the
// embeddings lane reads its projection.
func (l providerLift) upstream(label string, lane ProviderConfig, routing *OpenRouterRouting, embeddings bool) {
	settings := l.providers[lane.Provider]
	if embeddings && sameEmbeddingsFilter(routing, projectEmbeddings(settings.Upstream)) {
		return
	}
	if !embeddings && reflect.DeepEqual(routing, UpstreamPreferencesFor(ProviderConfig{
		Provider: lane.Provider, BaseURL: settings.BaseURL, Routing: settings.Upstream.clone(),
	})) {
		return
	}
	if settings.Upstream == nil && (!embeddings || !l.tierBound[lane.Provider]) {
		settings.Upstream = routing
		l.providers[lane.Provider] = settings
		l.upstreamFrom[lane.Provider] = label
		return
	}
	l.warn("ai: routing: lane upstream preferences differ from its provider's; the provider's win", lane.Provider, l.upstreamFrom, label)
}

// sameEmbeddingsFilter treats `{}` and nil alike: on the embeddings lane, which
// takes no product default, both send no provider object.
func sameEmbeddingsFilter(lane, resolved *OpenRouterRouting) bool {
	if lane.IsEmpty() && resolved.IsEmpty() {
		return true
	}
	return reflect.DeepEqual(projectEmbeddings(lane), resolved)
}

func (l providerLift) warn(msg, provider string, from map[string]string, dropped string) {
	kept := from[provider]
	if kept == "" {
		kept = providerEntryLabel
	}
	l.log.Warn(msg, "provider", provider, "kept_from", kept, "dropped", dropped)
}

// resolveProviders fills every lane's host and upstream preferences from its
// provider. Tiers take the block as written, so an absent one still receives the
// product default afterwards; embeddings take only its host filters. The tier's
// own reasoning effort is merged later, by applyTierEffort.
func (cfg RoutingConfig) resolveProviders() RoutingConfig {
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for tier, lane := range cfg.Tiers {
		settings := cfg.Providers[lane.Provider]
		lane.BaseURL, lane.Routing = settings.BaseURL, settings.Upstream.clone()
		tiers[tier] = lane
	}
	cfg.Tiers = tiers
	settings := cfg.Providers[cfg.Embeddings.Provider]
	cfg.Embeddings.BaseURL, cfg.Embeddings.Routing = settings.BaseURL, projectEmbeddings(settings.Upstream)
	if cfg.Decisions != nil {
		decisions := *cfg.Decisions
		decisions.BaseURL = cfg.Providers[decisions.Provider].BaseURL
		cfg.Decisions = &decisions
	}
	return cfg
}

// applyTierEffort merges each tier's reasoning effort into its resolved
// preferences, after the product default, so a tier on a provider with no
// upstream block keeps the default and still caps its thinking. A tier no
// preference reaches keeps none; validate refuses the effort it declared.
func (cfg *RoutingConfig) applyTierEffort() {
	for tier, lane := range cfg.Tiers {
		if lane.ReasoningEffort == "" || !UpstreamPreferencesApply(lane) {
			continue
		}
		if lane.Routing == nil {
			lane.Routing = &OpenRouterRouting{}
		}
		lane.Routing.ReasoningEffort = lane.ReasoningEffort
		cfg.Tiers[tier] = lane
	}
}

// canonical is the document as stored: providers hold every host and upstream
// block, and no lane carries the resolved copies.
func (cfg RoutingConfig) canonical() RoutingConfig {
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for tier, lane := range cfg.Tiers {
		lane.BaseURL, lane.Routing = "", nil
		tiers[tier] = lane
	}
	if cfg.Tiers != nil {
		cfg.Tiers = tiers
	}
	cfg.Embeddings.BaseURL, cfg.Embeddings.Routing = "", nil
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
		return fmt.Errorf("ai: routing config: providers: %s: `upstream` names OpenRouter's own upstream-selection fields and base_url is not an OpenRouter host; remove the block, or point the provider at the broker",
			name)
	}
	if upstream.ReasoningEffort != "" {
		return fmt.Errorf("ai: routing config: providers: %s: `upstream` takes no reasoning_effort — how hard a model thinks is the tier's; set reasoning_effort on the tier", name)
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
