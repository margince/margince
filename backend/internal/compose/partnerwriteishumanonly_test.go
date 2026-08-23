// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Setting a partner's terms is a human act.
//
// `margin_tier` is the rate the commission ledger multiplies a won deal by, so
// this one route decides what a partner is paid on every future deal. The
// contract's own precedent for that shape is `decideCommissionEntry`, which is
// human-only for the same reason.
//
// The route reached main declaring `update_record` at the auto-execute tier.
// Nothing served it — `update_record`'s enum excludes partner and the provider
// refuses it — so it read as dead. It was not: `Access: tool` admits an agent
// principal on the REST route whatever the tool surface does, because a
// passport is a REST credential too (ADR-0055). A mapping no tool honoured was
// an open, unstaged write, and the gap between "the tool refuses this" and
// "nobody can do this" is exactly what a reader assumes away.
//
// This pins the conclusion rather than the wording: if somebody reopens the
// route to agents, they change this test and state the tier deliberately.

import "testing"

const upsertPartnerRoute = "PUT /v1/organizations/{id}/partner"

func TestSettingPartnerTermsIsHumanOnly(t *testing.T) {
	policy, declared := agentPolicies[upsertPartnerRoute]
	if !declared {
		t.Fatalf("%s has no agent policy — every mutating operation declares one, so this route "+
			"was renamed or removed and this test needs to follow it", upsertPartnerRoute)
	}
	if policy.Access != accessHumanOnly {
		t.Errorf("%s admits an agent as %q, want %q — margin_tier is what a partner is paid, "+
			"and reopening it to agents is a decision that comes with a tier",
			upsertPartnerRoute, policy.Access, accessHumanOnly)
	}
	// human-only carries no tool mapping. A leftover one is how the previous
	// declaration looked dead while still admitting an agent.
	if policy.Tool != "" || policy.Tier != "" || policy.Scope != "" {
		t.Errorf("%s is human-only but still carries tool=%q tier=%q scope=%q — a human-only route "+
			"maps to no tool, and a stale mapping is what made the last one unreadable",
			upsertPartnerRoute, policy.Tool, policy.Tier, policy.Scope)
	}
}

// The read side stays open: an agent may look a partner up, and only the write
// is closed. Asserting both together stops the fix being over-applied.
func TestReadingAPartnerStaysOpenToAgents(t *testing.T) {
	policy, declared := agentPolicies["GET /v1/organizations/{id}/partner"]
	if !declared {
		t.Fatal("GET /v1/organizations/{id}/partner has no agent policy")
	}
	if policy.Access != accessTool {
		t.Errorf("reading a partner is %q, want %q — the write was closed, not the read",
			policy.Access, accessTool)
	}
	if policy.Tool != "read_record" {
		t.Errorf("reading a partner maps to tool %q, want read_record", policy.Tool)
	}
}
