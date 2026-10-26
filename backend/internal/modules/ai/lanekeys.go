// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
)

// lanesNeedingAKey reports the saved bindings the adapters refuse to build for
// want of a credential.
//
// A save naming such a vendor is accepted, and the router then refuses to
// rebind and keeps serving what it served before. The read has to say so,
// because a routing screen showing only what was saved names a model that is
// not answering.
//
// Asked of the adapter rather than of a provider list, so the answer is the
// one the router will reach. The lane is named as the router names it, so a
// reader can line this up against a health rung. A vendor that takes no key, or whose key is sent
// only when set, builds without one and never appears here.
func lanesNeedingAKey(cfg RoutingConfig, keys config.Lookup) []crmcontracts.AiLaneNeedingAKey {
	// Empty rather than nil: the contract declares an array, and a nil slice
	// would serve `null` to a reader that cannot iterate it.
	out := make([]crmcontracts.AiLaneNeedingAKey, 0)
	add := func(lane, provider, model string, err error) {
		if !isKeyFault(err) {
			return
		}
		out = append(out, crmcontracts.AiLaneNeedingAKey{
			Lane: lane, Provider: provider, Model: &model, KeyEnv: cloudKeyEnv[provider],
		})
	}
	for _, tier := range cfg.sortedTiers() {
		binding := cfg.Tiers[tier]
		_, err := SelectBrain(binding, keys)
		add(string(tier), binding.Provider, binding.Model, err)
	}
	_, embedErr := SelectBrain(cfg.Embeddings.ProviderConfig, keys)
	add(string(TierEmbedLane), cfg.Embeddings.Provider, cfg.Embeddings.Model, embedErr)
	if cfg.Decisions != nil {
		_, decideErr := selectDecider(*cfg.Decisions, keys)
		add(string(TierDecideLane), cfg.Decisions.Provider, cfg.Decisions.Model, decideErr)
	}
	return out
}
