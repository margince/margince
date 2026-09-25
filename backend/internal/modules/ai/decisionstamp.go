// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// decisionSkipFor is why the bound decisions lane does not answer task, or ""
// when it may — subject still to a certification row for the site asking,
// which is read per call because it is per site.
//
// The rule is deliberately small: a bound lane serves every task, except that
// a local-only task takes only a local provider. Its ladder was narrowed to
// same-host rungs because its prompt carries mail nobody agreed to send off
// the machine, and a decision question built from the same inputs carries the
// same mail.
func decisionSkipFor(lane *DecisionsConfig, task Task) string {
	if lane == nil {
		return DecisionSkipUnbound
	}
	if d, _ := providerByName(lane.Provider); LocalOnly(task) && !d.local {
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
			Processing: decisionProcessing(lane.Provider),
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
// two words: a local provider runs on an endpoint the operator configured,
// and any other reaches a vendor's cloud.
func decisionProcessing(provider string) string {
	if d, _ := providerByName(provider); d.local {
		return "configured_endpoint"
	}
	return "cloud_provider"
}
