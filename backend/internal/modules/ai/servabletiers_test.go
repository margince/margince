// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "testing"

func TestServableTiersReachesTheDegradeClosure(t *testing.T) {
	// capture_confidentiality_verdict is local-only, and local_small degrades to
	// itself, so its servable set is exactly its ladder.
	got := ServableTiers(TaskCaptureConfidentialityVerdict)
	if len(got) != 1 || got[0] != TierLocalSmall {
		t.Errorf("ServableTiers(capture_confidentiality_verdict) = %v, want [local_small]", got)
	}
	if lead := LeadingTier(TaskCaptureConfidentialityVerdict); lead != TierLocalSmall {
		t.Errorf("LeadingTier = %q, want local_small", lead)
	}
	// draft_reply's ladder never names local_small, but cheap_cloud degrades to
	// it — so a model bound there can serve the task. This is the case a
	// ladder-only reading misses.
	got = ServableTiers(TaskDraftReply)
	if !containsTier(got, TierLocalSmall) {
		t.Errorf("ServableTiers(draft_reply) = %v, must include local_small via the cheap_cloud degrade", got)
	}
	if got[0] != TierCheapCloud {
		t.Errorf("ServableTiers(draft_reply)[0] = %q, want the leading ladder rung cheap_cloud", got[0])
	}
}

// A task with no ladder is not routable, and both helpers say so rather than
// inventing a rung. nl_search is declared `planned` and ships no ladder.
func TestTierResolutionOnATaskWithNoLadder(t *testing.T) {
	unrouted := Task("not_a_task_this_build_declares")
	if lead := LeadingTier(unrouted); lead != "" {
		t.Errorf("LeadingTier(unrouted) = %q, want the empty tier", lead)
	}
	if tiers := ServableTiers(unrouted); tiers != nil {
		t.Errorf("ServableTiers(unrouted) = %v, want nil — no ladder means no rung can serve it", tiers)
	}
}
