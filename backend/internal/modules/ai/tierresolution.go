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
