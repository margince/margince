// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
)

// labelledBinding is one provider a document binds, under the name a refusal
// uses for it.
type labelledBinding struct {
	label    string
	provider string
}

// providerBindingsOf lists every provider a config binds.
//
// Derived from the fields that carry a binding rather than from a list. A site
// this misses is a binding the save accepts and the stack cannot run.
func providerBindingsOf(cfg RoutingConfig) []labelledBinding {
	cfg = cfg.canonical().resolveProviders()
	var out []labelledBinding
	for _, tier := range cfg.sortedTiers() {
		if p := cfg.Tiers[tier].Provider; p != "" {
			out = append(out, labelledBinding{label: "tier " + string(tier), provider: p})
		}
	}
	if p := cfg.Embeddings.Provider; p != "" {
		out = append(out, labelledBinding{label: "embeddings", provider: p})
	}
	if cfg.Decisions != nil && cfg.Decisions.Provider != "" {
		out = append(out, labelledBinding{label: "decisions", provider: cfg.Decisions.Provider})
	}
	return out
}

// refuseKeylessBindings refuses a save that binds a provider holding no key.
//
// The stack cannot rebind one: it refuses every 30 s and keeps serving what it
// had, while the routing read answers the saved binding. Only what the save
// introduces is checked, as the Vertex probe is, so a key removed later leaves
// the document editable.
func (s *RoutingStore) refuseKeylessBindings(ctx context.Context, stored, next RoutingConfig) error {
	// Keyed by LANE and provider, not by provider alone. A tier already bound
	// to a keyless provider is left alone. That must not exempt the same
	// provider arriving on a lane which did not carry it.
	bound := map[labelledBinding]bool{}
	for _, b := range providerBindingsOf(stored) {
		bound[b] = true
	}
	keys := s.resolvedKeys(ctx)
	for _, b := range providerBindingsOf(next) {
		if bound[b] || cloudKeyEnv[b.provider] == "" {
			continue
		}
		if cloudKey(b.provider, keys) != "" {
			continue
		}
		return fmt.Errorf("ai: routing config: %s: %w", b.label, byokKeyRequired(b.provider))
	}
	return nil
}
