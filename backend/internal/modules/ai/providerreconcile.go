// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// The codes a routing write is refused with when the refusal is about where a
// provider is configured, so a client can send the admin to the provider sheet
// rather than the binding. The settings package owns only the generic code.
const (
	// CodeMovedToProvider: a lane names a host or pins its provider already
	// holds differently; they are changed on the provider now.
	CodeMovedToProvider = "moved_to_provider"
	// CodeNoHost: a provider that cannot be dialled without a host has none
	// while some lane binds it.
	CodeNoHost = "no_host"
)

// reconcileLaneProviderFields settles what an old client writes on its lanes —
// the host each tier and the decisions lane named, a tier's Vertex location,
// and the connection keys inside a tier's routing — against the provider that
// owns them now. A host or location its provider lacks is lifted onto it, an
// equal value is cleared, and a different one is refused naming the lane.
// Connection keys are never lifted on a write: they decide who may read every
// tier's requests, so they are set on the connection or not at all.
//
// A nil next.Providers keeps the stored providers: a client that predates them
// never sends the map. A map it does send, even empty, is the whole of it.
//
// The embeddings lane is left to the lift: its host and pins are its own
// server's when they differ (D9), not a disagreement.
func (cfg RoutingConfig) reconcileLaneProviderFields(stored RoutingConfig) (RoutingConfig, error) {
	providers := cfg.Providers
	if providers == nil {
		providers = stored.canonical().Providers
	}
	r := laneReconcile{providers: make(map[string]ProviderSettings, len(providers))}
	for name, entry := range providers {
		entry.Upstream = entry.Upstream.clone()
		r.providers[name] = entry
	}
	tiers := make(map[Tier]ProviderConfig, len(cfg.Tiers))
	for _, tier := range cfg.sortedTiers() {
		lane, err := r.tier(tierLabel(tier), TierRoutingPath(tier), cfg.Tiers[tier])
		if err != nil {
			return RoutingConfig{}, err
		}
		tiers[tier] = lane
	}
	if cfg.Tiers != nil {
		cfg.Tiers = tiers
	}
	if cfg.Decisions != nil {
		decisions := *cfg.Decisions
		if err := r.host(decisionsLaneLabel, decisions.Provider, decisions.BaseURL); err != nil {
			return RoutingConfig{}, err
		}
		decisions.BaseURL = ""
		cfg.Decisions = &decisions
	}
	cfg.Providers = r.providers
	if len(cfg.Providers) == 0 {
		cfg.Providers = nil
	}
	return cfg, nil
}

type laneReconcile struct{ providers map[string]ProviderSettings }

func (r laneReconcile) tier(label, path string, lane ProviderConfig) (ProviderConfig, error) {
	if err := r.host(label, lane.Provider, lane.BaseURL); err != nil {
		return ProviderConfig{}, err
	}
	if pins := lane.Routing.pins(); pins != nil {
		if held := r.providers[lane.Provider].Upstream.pins(); held == nil || !samePins(held, pins) {
			return ProviderConfig{}, refuseConnectionKeysOnTier(path, pins)
		}
		lane.Routing = lane.Routing.withoutPins()
	}
	if err := r.location(label, lane.Provider, lane.Location); err != nil {
		return ProviderConfig{}, err
	}
	lane.BaseURL, lane.Location = "", ""
	return lane, nil
}

func (r laneReconcile) location(label, provider, location string) error {
	entry := r.providers[provider]
	switch {
	case location == "" || location == entry.Location:
		return nil
	case entry.Location == "":
		entry.Location = location
		r.providers[provider] = entry
		return nil
	}
	return movedToProvider(label, "the location is set on the provider now; change it", provider)
}

func (r laneReconcile) host(label, provider, host string) error {
	entry := r.providers[provider]
	switch {
	case host == "" || sameHost(provider, entry.BaseURL, host):
		return nil
	case entry.BaseURL == "":
		entry.BaseURL = host
		r.providers[provider] = entry
		return nil
	}
	return movedToProvider(label, "the host is set on the provider now; change it", provider)
}

// refuseConnectionKeysOnTier refuses the connection keys a tier wrote, one
// fault per key. They are accepted only as an echo of the connection's own
// value, which is what a client writing back a resolved binding sends.
func refuseConnectionKeysOnTier(path string, pins *OpenRouterRouting) error {
	written, err := json.Marshal(pins.Provider)
	if err != nil {
		return err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(written, &keys); err != nil {
		return err
	}
	var faults routingFaults
	for _, key := range connectionKeys {
		if _, ok := keys[key]; ok {
			faults = append(faults, routingFault{Path: path + ".provider." + key, Code: CodeMovedToProvider, Message: connectionKeyOnTier})
		}
	}
	return faults
}

// connectionKeyOnTier is the refusal for a host filter or privacy key on a tier.
const connectionKeyOnTier = "is set on the connection, under OpenRouter settings, and applies to every tier. Remove it here."

func movedToProvider(label, what, provider string) error {
	return settings.InvalidValue{
		Setting: RoutingKey, Code: CodeMovedToProvider,
		Reason: fmt.Sprintf("%s: %s with PUT /ai/provider-settings/%s", label, what, provider),
	}
}

// samePins compares two sets of connection keys: a host list is a set, so
// its order says nothing.
func samePins(a, b *OpenRouterRouting) bool {
	normal := func(r *OpenRouterRouting) OpenRouterProvider {
		p := r.pins().Provider
		p.Only, p.Ignore = slices.Sorted(slices.Values(p.Only)), slices.Sorted(slices.Values(p.Ignore))
		return p
	}
	return reflect.DeepEqual(normal(a), normal(b))
}

// missingHostError is a lane refusal whose cause is a provider with nowhere to
// dial, so a write can answer it as no_host naming every lane that binds it.
// Its message is the lane rule's own, which a routing file's reader sees.
type missingHostError struct {
	provider string
	reason   string
}

func (e missingHostError) Error() string { return e.reason }

// chatHostMissing is whether a chat lane on provider at host cannot be
// served: the OpenAI wire has no default host to fall back on.
func chatHostMissing(provider, host string) bool {
	return provider == providerOpenAICompatible && strings.TrimSpace(host) == ""
}

// decisionHostMissing is whether a decisions lane on provider at host cannot
// be served: a decision adapter with no compiled endpoint needs one written.
func decisionHostMissing(provider, host string) bool {
	d, _ := providerByName(provider)
	return defaulted(host, d.defaultEndpoint) == ""
}

// routingRefusal turns a refused write into what the caller is told: each bad
// key by its path when the refusal names them, no_host when the cause is an
// unhosted provider, the generic code otherwise.
func (cfg RoutingConfig) routingRefusal(err error) error {
	var invalid settings.InvalidValue
	if errors.As(err, &invalid) {
		return invalid
	}
	var faults routingFaults
	if errors.As(err, &faults) {
		return faults
	}
	var missing missingHostError
	if errors.As(err, &missing) {
		return settings.InvalidValue{
			Setting: RoutingKey, Code: CodeNoHost,
			Reason: fmt.Sprintf("%s is bound by %s; give it a host or rebind those lanes first",
				missing.provider, strings.Join(cfg.unhostedLanes(missing.provider), ", ")),
		}
	}
	return settings.InvalidValue{Setting: RoutingKey, Code: settings.CodeInvalidValue, Reason: err.Error()}
}

// unhostedLanes names, in lift order, the lanes on provider that resolve to no
// host they can be served at.
func (cfg RoutingConfig) unhostedLanes(provider string) []string {
	resolved := cfg.canonical().resolveProviders()
	var lanes []string
	for _, tier := range resolved.sortedTiers() {
		if lane := resolved.Tiers[tier]; lane.Provider == provider && chatHostMissing(provider, lane.BaseURL) {
			lanes = append(lanes, tierLabel(tier))
		}
	}
	if e := resolved.Embeddings; e.Provider == provider && chatHostMissing(provider, e.BaseURL) {
		lanes = append(lanes, embeddingsLaneLabel)
	}
	if d := resolved.Decisions; d != nil && d.Provider == provider && decisionHostMissing(provider, d.BaseURL) {
		lanes = append(lanes, decisionsLaneLabel)
	}
	return lanes
}
