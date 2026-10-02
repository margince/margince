// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"reflect"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// decisionSkipFor is why the bound decisions lane does not answer task, or ""
// when it may.
//
// The local-only check reads localOnlyAdmits, the same predicate the ladder's
// servableLadder reads — one place, so a restored local-only guarantee (or a
// further reverted one) reaches both without a second edit.
func decisionSkipFor(lane *DecisionsConfig, task Task) string {
	if lane == nil {
		return DecisionSkipUnbound
	}
	if LocalOnly(task) && !localOnlyAdmits(task, lane.isLocal()) {
		return DecisionSkipLocalOnly
	}
	return ""
}

// decisionRoute fills a feature row's decision fields. A feature that
// declares no decision form carries none; one that does says whether the lane
// answers it first and, when not, why — in the order an operator would fix
// it: bind a lane, keep local data local. No certification row is required —
// a bound lane serves every site its local-only rule admits, the same as any
// other task's ladder rung.
func decisionRoute(row *crmcontracts.AiFeatureRoute, cfg RoutingConfig, task Task, blocked bool) {
	if !TaskDecides(task) {
		return
	}
	// The lane reads its host from its provider, as every planned lane does.
	cfg = cfg.canonical().resolveProviders()
	skip := decisionSkipFor(cfg.Decisions, task)
	if lane := cfg.Decisions; lane != nil {
		row.DecisionCandidate = &crmcontracts.AiRouteCandidate{
			Tier: string(TierDecideLane), Provider: lane.Provider, Model: lane.Model,
			Processing: decisionProcessing(*lane),
		}
	}
	if skip != "" {
		row.DecisionSkipReason = &skip
		return
	}
	// A deferred feature asks nothing at all, so the lane does not answer it
	// either; the row's impact already says why.
	row.DecisionFirst = !blocked
}

// decisionProcessing is where the lane's inference happens, in the preview's
// two words: a local lane runs on an endpoint the operator configured, and any
// other reaches a cloud. The same isLocal the local-only rule reads.
func decisionProcessing(lane DecisionsConfig) string {
	if lane.isLocal() {
		return "configured_endpoint"
	}
	return "cloud_provider"
}

// decisionLeadChanged reports whether the model that answers task FIRST moved
// between two routing documents on the decision lane alone: the lane started or
// stopped answering it, or answers it on another binding. after is the
// proposed document's row, already stamped. A lane that answers the feature in
// neither document changes nothing a caller sees, whatever its skip reason.
func decisionLeadChanged(before, proposed RoutingConfig, task Task, beforeBlocked bool, after crmcontracts.AiFeatureRoute) bool {
	var was crmcontracts.AiFeatureRoute
	decisionRoute(&was, before, task, beforeBlocked)
	if was.DecisionFirst != after.DecisionFirst {
		return true
	}
	return after.DecisionFirst && !reflect.DeepEqual(before.Decisions, proposed.Decisions)
}
