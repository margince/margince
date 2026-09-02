// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Which TIERS can serve a task, and which one leads.
//
// Split from routing.go because it answers a different question: that file owns
// the tier→model binding an operator writes, while this one reads the task
// contract's ladders and the degrade table to say which rungs a task can reach.
// The certification lane and the readiness report are the callers — both need to
// resolve a task to the model that would actually serve it, and neither should
// re-derive a ladder this package owns.

// LeadingTier is the rung a task is served on when nothing has gone wrong: the
// first entry of its ladder. Zero value for a task with no ladder, which
// TaskLadder's callers already treat as "not a routable task".
func LeadingTier(task Task) Tier {
	ladder := taskLadders[task]
	if len(ladder) == 0 {
		return ""
	}
	return ladder[0]
}

// EffectiveModel is the model a binding actually serves with, including the
// defaults the client builder supplies for the providers that have one.
//
// It exists because "no model in the routing document" does NOT mean "no model
// serves": SelectBrain defaults ollama to defaultOllamaModel and vLLM to
// defaultVLLMModel, so a tier written `{provider: ollama}` is served — and a
// caller that read Model directly would report a task the router runs every day
// as having no model bound.
//
// This and the client builder are two writers of one rule. Held by:
// TestEffectiveModelIsWhatTheClientBuilderWouldServe
// (internal/modules/ai/effectivemodeldefaults_test.go), which drives the real
// builder and reads the default off the client it produced rather than
// comparing two constants that happen to agree today.
func EffectiveModel(binding ProviderConfig) string {
	switch binding.Provider {
	case providerOllama:
		return defaulted(binding.Model, defaultOllamaModel)
	case providerVLLM:
		return defaulted(binding.Model, defaultVLLMModel)
	default:
		return binding.Model
	}
}

// IsBound says whether a tier binding serves anything.
//
// A tier present in a routing map with no provider, or with no model AND no
// provider default, is NOT bound: the router builds no client from it. It is a
// function rather than two comparisons at each call site because three callers
// reasoning about what answers had each written the same pair, and a fourth
// would have written it again.
func IsBound(binding ProviderConfig) bool {
	return binding.Provider != "" && EffectiveModel(binding) != ""
}

// FirstBoundTier is the rung a deployment actually serves a task on: the first
// rung of its ladder that the deployment BINDS, with the binding found there.
//
// First bound, not simply first, because that is what production does. The
// router filters a task's ladder to the rungs it has a client for and serves the
// leading survivor (attemptLadder) — so a deployment that leaves premium unbound
// still serves a premium-led task on cheap_cloud. A caller that read ladder[0]
// would name a model the router never reaches, and then report on a deployment
// nobody is running.
//
// A tier present in the map with no provider or no model is NOT bound: the
// router builds no client from it, and neither may a caller reasoning about what
// answers.
func FirstBoundTier(routing RoutingConfig, task Task) (ProviderConfig, Tier, bool) {
	for _, tier := range taskLadders[task] {
		binding, bound := routing.Tiers[tier]
		if !bound || !IsBound(binding) {
			continue
		}
		// The effective model, not the document's: a caller joining certification
		// records on the model must look up what ANSWERED, and for ollama and
		// vLLM that can be a default the routing file never spells.
		binding.Model = EffectiveModel(binding)
		return binding, tier, true
	}
	return ProviderConfig{}, "", false
}

// ServableTiers returns the tiers that can end up serving a task: its ladder,
// plus the transitive closure of degradeTo over those rungs.
//
// The closure is the part a caller forgets and must not. draft_reply's ladder is
// [cheap_cloud, premium] and cheap_cloud degrades to local_small, so the model
// bound at local_small can serve draft_reply under budget pressure while the
// ladder never names that rung. A certification report built from the ladder
// alone would call the task covered with its degrade path unmeasured — an answer
// a real deployment can reach, from a model nothing graded.
//
// Deterministic order: ladder rungs first, in ladder order, then the rungs only
// the closure reaches, in AllTiers order. A caller printing this gets the same
// row twice running.
func ServableTiers(task Task) []Tier {
	ladder := taskLadders[task]
	if len(ladder) == 0 {
		return nil
	}
	seen := make(map[Tier]bool, len(ladder))
	out := make([]Tier, 0, len(ladder))
	for _, tier := range ladder {
		if !seen[tier] {
			seen[tier] = true
			out = append(out, tier)
		}
	}
	// Walk the closure from every rung. degradeTo maps local_small to itself, so
	// a plain loop would not terminate on its own — the seen set is what ends it,
	// and it is also what makes a cycle anywhere in the table safe.
	for _, tier := range ladder {
		for {
			next, ok := degradeTo[tier]
			if !ok || seen[next] {
				break
			}
			seen[next] = true
			tier = next
		}
	}
	for _, tier := range AllTiers() {
		if seen[tier] && !containsTier(out, tier) {
			out = append(out, tier)
		}
	}
	return out
}

func containsTier(tiers []Tier, want Tier) bool {
	for _, tier := range tiers {
		if tier == want {
			return true
		}
	}
	return false
}
