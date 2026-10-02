// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// What each attempt was sent with, recorded on the config dimension row its
// ai_call row points at, so a call can be read back with the routing block,
// the thinking level and the deadline that shaped it.

import (
	"encoding/json"
	"sync"
	"time"
)

// sentParams is a completion or decision attempt's provider_params. Each field
// is fixed per (binding, task, tier) — never a value of one call — so the
// snapshot it hashes into is a small set of rows, not one per call.
type sentParams struct {
	EmbedDimensions int                  `json:"embed_dimensions"`
	Provider        *OpenRouterProvider  `json:"provider,omitempty"`
	Reasoning       *OpenRouterReasoning `json:"reasoning,omitempty"`
	ThinkingLevel   string               `json:"thinking_level,omitempty"`
	DeadlineMs      int64                `json:"deadline_ms"`
}

// sentSnapshotKey names one memoized snapshot.
type sentSnapshotKey struct {
	tier     Tier
	kind     string
	thinking string
	deadline time.Duration
}

// sentSnapshots memoizes a binding's snapshots, built on first use. A pointer,
// so the binding value that carries it can be copied before it is installed.
type sentSnapshots struct {
	byKey sync.Map
}

// snapshotFor is the config snapshot an attempt of kind on tier is recorded
// under, given what the task's calls are sent with. An embedding keeps the
// binding's own snapshot: it is sent no routing, thinking or ladder deadline.
func (b *binding) snapshotFor(tier Tier, kind string, task EffectiveTask) ConfigSnapshot {
	if kind == callKindEmbedding || b.sent == nil {
		return b.configSnapshot
	}
	key := sentSnapshotKey{tier: tier, kind: kind, thinking: task.Thinking, deadline: task.AttemptTimeout}
	if kind == callKindDecision {
		key = sentSnapshotKey{tier: tier, kind: kind, deadline: task.DecisionTimeout}
	}
	if stored, ok := b.sent.byKey.Load(key); ok {
		if snap, ok := stored.(ConfigSnapshot); ok {
			return snap
		}
	}
	snap := b.buildSnapshot(key)
	b.sent.byKey.Store(key, snap)
	return snap
}

func (b *binding) buildSnapshot(key sentSnapshotKey) ConfigSnapshot {
	params := sentParams{EmbedDimensions: b.embedDims, DeadlineMs: key.deadline.Milliseconds(), ThinkingLevel: key.thinking}
	if routing, broker := b.tierRouting[key.tier]; broker && key.kind == callKindCompletion {
		params.Provider, params.Reasoning = routing.providerWire(), routing.reasoningWire()
		if key.thinking != "" {
			params.Reasoning = &OpenRouterReasoning{Effort: key.thinking}
		}
	}
	// A struct of plain values and the routing types, which marshal cannot fail
	// on; the same reasoning as embedDimensionsParams.
	raw, _ := json.Marshal(params) //nolint:errchkjson // plain values; marshal cannot fail
	snap := ConfigSnapshot{
		TaskContractHash: TaskContractHash, RoutingConfigHash: b.configSnapshot.RoutingConfigHash, ProviderParams: raw,
	}
	snap.Hash = computeConfigHash(snap.TaskContractHash, snap.RoutingConfigHash, snap.PromptVersion, snap.ProviderParams)
	return snap
}

// brokerRouting is each broker tier's resolved routing, which is what its
// adapter renders onto the wire.
func brokerRouting(cfg RoutingConfig) map[Tier]*OpenRouterRouting {
	out := map[Tier]*OpenRouterRouting{}
	for tier, lane := range cfg.Tiers {
		if UpstreamPreferencesApply(lane) {
			out[tier] = UpstreamPreferencesFor(lane)
		}
	}
	return out
}
