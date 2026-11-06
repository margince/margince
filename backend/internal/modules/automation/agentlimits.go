// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent may set up a rule, within limits a user's own hand does not meet. A
// rule runs on with nobody watching, and the seeded ones tell owners what happened.

func agentRuleLimit(code, detail string) error {
	return &httperr.DetailedError{Status: http.StatusForbidden, Code: code, Detail: detail}
}

func callingAgent(ctx context.Context) (principal.Principal, bool) {
	actor, ok := principal.Actor(ctx)
	return actor, ok && actor.Type == principal.PrincipalAgent
}

// seededRow reports the rule the product itself started: a seeded key with no owner.
// A rule someone created from that key, or took on, has an owner and is theirs.
func seededRow(entry CatalogEntry, rule Automation) bool {
	return entry.Seeded && rule.OwnerID == nil
}

// seededLimit refuses quieting a rule the product starts with.
func seededLimit() error {
	return agentRuleLimit("agent_quiets_no_seeded_rule",
		"this seeded rule tells owners what happened, so an agent does not pause, retune or archive it; ask the user to do it in the app")
}

// refuseAgentRuleChange holds an agent's update to its limits. A rule whose step
// composes a draft spends a model call each time it fires. So enabling one takes
// the enrich scope the agent's own AI work takes.
func refuseAgentRuleChange(ctx context.Context, before Automation, in UpdateAutomationInput) error {
	actor, isAgent := callingAgent(ctx)
	if !isAgent {
		return nil
	}
	if in.OwnerID != nil && (before.OwnerID == nil || *in.OwnerID != *before.OwnerID) {
		return agentRuleLimit("agent_hands_on_no_rule",
			"an agent does not change who owns a rule, because it runs with its owner's authority; ask the user to do it in the app")
	}
	entry, known := CatalogEntryByKey(before.Key)
	if !known {
		return nil
	}
	turnsOff := in.Enabled != nil && !*in.Enabled && before.Enabled
	retunes := in.Params != nil && !sameParams(before.Params, in.Params)
	if seededRow(entry, before) && (turnsOff || retunes) {
		return seededLimit()
	}
	if in.Enabled != nil && *in.Enabled && entry.Action == string(ActionTypeDraftEmail) && !actor.Scopes.Has(principal.ScopeEnrich) {
		return agentRuleLimit("agent_needs_enrich_for_rule",
			"this rule writes a draft with AI each time it fires, so enabling it needs a passport with the enrich scope")
	}
	return nil
}

func refuseAgentArchivingSeeded(ctx context.Context, before Automation) error {
	if _, isAgent := callingAgent(ctx); !isAgent {
		return nil
	}
	if entry, known := CatalogEntryByKey(before.Key); known && seededRow(entry, before) {
		return seededLimit()
	}
	return nil
}
