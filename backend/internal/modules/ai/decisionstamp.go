// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"reflect"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// decisionSkipFor is why the bound decisions lane does not answer task, or ""
// when it may — subject still to a certification row for the site asking,
// which is read per call because it is per site.
//
// The rule is deliberately small: a bound lane serves every task, except that
// a local-only task takes only a local lane. Its ladder was narrowed to
// same-host rungs because its prompt carries mail nobody agreed to send off
// the machine, and a decision question built from the same inputs carries the
// same mail.
func decisionSkipFor(lane *DecisionsConfig, task Task) string {
	if lane == nil {
		return DecisionSkipUnbound
	}
	if LocalOnly(task) && !lane.isLocal() {
		return DecisionSkipLocalOnly
	}
	return ""
}

// anySiteCertified reports whether some site of task has a certification row
// for the lane's provider and model: the route preview's "would the lane
// answer this feature at all", which the runtime asks per site.
func anySiteCertified(task Task, lane DecisionsConfig, certified func(DecisionCertKey) bool) bool {
	for _, site := range SitesFor(task) {
		if certified(DecisionCertKey{Task: task, Site: site.Name, Provider: lane.Provider, Model: lane.Model}) {
			return true
		}
	}
	return false
}

// decisionRoute fills a feature row's decision fields. A feature that
// declares no decision form carries none; one that does says whether the lane
// answers it first and, when not, why — in the order an operator would fix
// it: bind a lane, keep local data local, certify the site.
func decisionRoute(row *crmcontracts.AiFeatureRoute, cfg RoutingConfig, task Task, blocked bool) {
	if !TaskDecides(task) {
		return
	}
	skip := decisionSkipFor(cfg.Decisions, task)
	if skip == "" && !anySiteCertified(task, *cfg.Decisions, decisionIsCertified) {
		skip = DecisionSkipUncertified
	}
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
