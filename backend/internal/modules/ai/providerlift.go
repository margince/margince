// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"cmp"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"sync"
)

// liftedRevisions holds each stored revision this process has lifted. The
// routing watcher re-reads the row on every recheck on every role, and a lift's
// warnings describe the row, so they are said once per revision; the set grows
// by one per saved edit.
var liftedRevisions sync.Map

// firstLiftOf reports whether this process has not lifted rev before, and
// records that it now has.
func firstLiftOf(rev string) bool {
	_, seen := liftedRevisions.LoadOrStore(rev, struct{}{})
	return !seen
}

// providerLift carries a lift's state across lanes: the providers being built,
// which lane decided each kept value (for the warning that names both), and the
// lanes that stated no pins of their own.
type providerLift struct {
	providers map[string]ProviderSettings
	hostFrom  map[string]string
	pinsFrom  map[string]string
	placeFrom map[string]string
	unpinned  map[string][]string
	tierBound map[string]bool
	// perLane is true for a document written before providers held hosts, where
	// an empty lane host meant the compiled default rather than the provider's.
	perLane bool
	log     *slog.Logger
}

// liftLaneProviderFields moves what lanes wrote about their provider — the
// shape every routing document had before providers held it — onto the
// provider. Tiers by name, then decisions, decide the host and the pins; the
// first wins and each disagreeing lane is one warning. The embeddings lane
// decides only for a provider nothing else binds, and otherwise keeps a host or
// pins that differ as its own override. A value equal to its provider's is
// cleared silently, so the lift is idempotent and safe over a resolved config.
func (cfg RoutingConfig) liftLaneProviderFields(log *slog.Logger) RoutingConfig {
	lift := providerLift{
		providers: maps.Clone(cfg.Providers), hostFrom: map[string]string{}, pinsFrom: map[string]string{}, placeFrom: map[string]string{},
		unpinned: map[string][]string{}, tierBound: map[string]bool{}, perLane: len(cfg.Providers) == 0, log: log,
	}
	if lift.providers == nil {
		lift.providers = map[string]ProviderSettings{}
	}
	for name, settings := range lift.providers {
		if settings.BaseURL != "" {
			lift.hostFrom[name] = providerEntryLabel
		}
		if settings.Upstream != nil {
			lift.pinsFrom[name] = providerEntryLabel
		}
		if settings.Location != "" {
			lift.placeFrom[name] = providerEntryLabel
		}
	}
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for _, tier := range cfg.sortedTiers() {
		lane := cfg.Tiers[tier]
		// A provider this build does not know cannot hold what its lane wrote;
		// the lane keeps it and fails when asked to serve, rather than the whole
		// document being refused with every other lane.
		if !knownProvider(lane.Provider) {
			tiers[tier] = lane
			continue
		}
		lift.tierBound[lane.Provider] = true
		tiers[tier] = lift.tier(tierLabel(tier), lane)
	}
	cfg.Tiers = tiers
	if cfg.Decisions != nil && knownProvider(cfg.Decisions.Provider) {
		decisions := *cfg.Decisions
		lift.tierBound[decisions.Provider] = true
		lift.host(decisionsLaneLabel, decisions.Provider, decisions.BaseURL)
		decisions.BaseURL = ""
		cfg.Decisions = &decisions
	}
	if knownProvider(cfg.Embeddings.Provider) {
		cfg.Embeddings.ProviderConfig = lift.embeddings(cfg.Embeddings.ProviderConfig)
		cfg.Embeddings.Location = lift.embeddingsLocation(cfg.Embeddings.Provider, cfg.Embeddings.Location)
	}
	lift.warnInherited()
	if len(lift.providers) == 0 {
		lift.providers = nil
	}
	cfg.Providers = lift.providers
	return cfg
}

// knownProvider reports whether this build knows a provider.
func knownProvider(provider string) bool {
	_, known := providerByName(provider)
	return known
}

// tier lifts one tier's host and pins and returns it with neither.
func (l providerLift) tier(label string, lane ProviderConfig) ProviderConfig {
	l.host(label, lane.Provider, lane.BaseURL)
	dropped := false
	if pins := lane.Routing.pins(); pins != nil {
		dropped = !l.pins(label, lane.Provider, pins)
	} else {
		l.unpinned[lane.Provider] = append(l.unpinned[lane.Provider], label)
	}
	lane.BaseURL, lane.Routing = "", lane.Routing.withoutPins()
	lane.Location = l.location(label, lane.Provider, lane.Location)
	// A block that held only pins no broker can honour is no declaration at all,
	// and a left-over `{}` would be refused on the same non-broker host.
	if dropped && lane.Routing.IsEmpty() {
		lane.Routing = nil
	}
	return lane
}

// host lets the first lane decide its provider's host — an empty one included,
// which is the compiled default — and warns for each later lane elsewhere. Once
// providers hold hosts, an empty lane host is the provider's and states nothing.
func (l providerLift) host(label, provider, baseURL string) {
	if baseURL == "" && !l.perLane {
		return
	}
	if _, decided := l.hostFrom[provider]; decided {
		if !sameHost(provider, l.providers[provider].BaseURL, baseURL) {
			l.warnKept("ai: routing: lane host differs from its provider's; the provider's wins", provider, l.hostFrom, label)
		}
		return
	}
	l.hostFrom[provider] = label
	if baseURL != "" {
		settings := l.providers[provider]
		settings.BaseURL = baseURL
		l.providers[provider] = settings
	}
}

// location lets the first tier that names one decide its provider's Vertex
// location, warns for each later tier naming another, and returns the tier's
// copy cleared.
func (l providerLift) location(label, provider, location string) string {
	if location == "" {
		return ""
	}
	if _, decided := l.placeFrom[provider]; !decided {
		l.placeFrom[provider] = label
		settings := l.providers[provider]
		settings.Location = location
		l.providers[provider] = settings
		return ""
	}
	if l.providers[provider].Location != location {
		l.warnKept("ai: routing: lane location differs from its provider's; the provider's wins", provider, l.placeFrom, label)
	}
	return ""
}

// embeddingsLocation is the embeddings lane's own location after the lift.
// Vertex serves an embedding model at fewer locations than a chat model, so
// the lane may sit elsewhere, as it may sit on a server of its own; it decides
// the provider's location only when neither a tier nor the stored entry has.
func (l providerLift) embeddingsLocation(provider, location string) string {
	if _, decided := l.placeFrom[provider]; !decided && !l.tierBound[provider] {
		return l.location(embeddingsLaneLabel, provider, location)
	}
	if location == l.providers[provider].Location {
		return ""
	}
	return location
}

// pins lets the first lane decide its provider's pins. Pins that cannot reach a
// broker are dropped with a warning rather than refused: the row was valid when
// stored, and refusing it here would take the installation's AI down at boot.
// It reports false for pins it dropped.
func (l providerLift) pins(label, provider string, pins *OpenRouterRouting) bool {
	settings := l.providers[provider]
	switch {
	case !providerFrontsBroker(provider, settings):
		l.log.Warn("ai: routing: lane upstream pins dropped; its provider's host is not OpenRouter", "provider", provider, "dropped", label)
		return false
	case settings.Upstream == nil:
		settings.Upstream = pins
		l.providers[provider] = settings
		l.pinsFrom[provider] = label
	case !reflect.DeepEqual(settings.Upstream.pins(), pins):
		l.warnKept("ai: routing: lane upstream pins differ from its provider's; the provider's win", provider, l.pinsFrom, label)
	}
	return true
}

// embeddings lifts the embeddings lane's host and pins onto a provider nothing
// else decided, and otherwise keeps whichever differs from the provider's as
// the lane's own override.
func (l providerLift) embeddings(lane ProviderConfig) ProviderConfig {
	lane.BaseURL = l.embeddingsHost(lane.Provider, lane.BaseURL)
	if lane.BaseURL == "" {
		lane.Routing = l.embeddingsPins(lane.Provider, lane.Routing)
	}
	return lane
}

// embeddingsHost is the embeddings lane's host override after the lift: empty
// when it is served at its provider's host.
func (l providerLift) embeddingsHost(provider, baseURL string) string {
	if _, decided := l.hostFrom[provider]; !decided && !l.tierBound[provider] {
		l.host(embeddingsLaneLabel, provider, baseURL)
	}
	host := l.providers[provider].BaseURL
	switch {
	case baseURL == "" && l.perLane && !sameHost(provider, host, ""):
		return compiledHost(provider)
	case sameEndpoint(baseURL, host):
		return ""
	}
	return baseURL
}

// embeddingsPins is the embeddings lane's routing after the lift, for a lane
// served at its provider's host.
func (l providerLift) embeddingsPins(provider string, routing *OpenRouterRouting) *OpenRouterRouting {
	pins := routing.pins()
	settings := l.providers[provider]
	switch {
	case pins == nil:
		l.unpinned[provider] = append(l.unpinned[provider], embeddingsLaneLabel)
		return routing
	case settings.Upstream == nil && !l.tierBound[provider] && providerFrontsBroker(provider, settings):
		if l.pins(embeddingsLaneLabel, provider, pins) {
			routing = routing.withoutPins()
		}
	case reflect.DeepEqual(settings.Upstream.pins(), pins):
		routing = routing.withoutPins()
	}
	// The embeddings lane takes no product default, so a block left with nothing
	// in it is the same request as no block at all.
	if routing.IsEmpty() {
		return nil
	}
	return routing
}

// warnInherited names each lane that stated no pins and is now served under
// pins another lane stated: a model the pinned hosts do not serve fails on
// every call, and this warning is the one place that says why.
func (l providerLift) warnInherited() {
	for _, provider := range slices.Sorted(maps.Keys(l.unpinned)) {
		from := l.pinsFrom[provider]
		if from == "" || from == providerEntryLabel {
			continue
		}
		for _, label := range l.unpinned[provider] {
			l.log.Warn("ai: routing: lane inherits upstream pins another lane stated", "provider", provider, "pinned_by", from, "inheriting", label)
		}
	}
}

func (l providerLift) warnKept(msg, provider string, from map[string]string, dropped string) {
	l.log.Warn(msg, "provider", provider, "kept_from", from[provider], "dropped", dropped)
}

// compiledHost is where a provider is dialled when no host is written: the
// local adapter's endpoint, the vendor's public API, or a decision adapter's.
// An unknown provider has none; validateProviderEntries refuses its name.
func compiledHost(provider string) string {
	d, known := providerByName(provider)
	if !known {
		return ""
	}
	return cmp.Or(d.defaultBaseURL, d.vendorBaseURL, d.defaultEndpoint)
}

// sameHost compares two hosts for one provider, reading an empty one as the
// compiled default it dials.
func sameHost(provider, a, b string) bool {
	orDefault := func(host string) string {
		if host == "" {
			return compiledHost(provider)
		}
		return host
	}
	return sameEndpoint(orDefault(a), orDefault(b))
}

func providerFrontsBroker(provider string, settings ProviderSettings) bool {
	return UpstreamPreferencesApply(ProviderConfig{Provider: provider, BaseURL: settings.BaseURL})
}
