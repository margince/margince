// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"strings"
)

// DecisionsConfig binds the decision-model lane: a provider, a model and an
// endpoint, and nothing a chat tier carries — the lane sends no attachments
// and takes no broker preferences, so it has no field to declare either.
//
// Optional. Absent, no task is asked a decision question and every call is
// exactly the ladder's; present, a task whose contract declares a decision
// form may be answered by it first (decisionstamp.go says which).
type DecisionsConfig struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
	BaseURL  string `yaml:"base_url" json:"base_url,omitempty"`
}

// validateDecisionsLane holds a bound lane to ValidateDecisionsLane; an
// unbound one has nothing to check.
func (cfg RoutingConfig) validateDecisionsLane() error {
	if cfg.Decisions == nil {
		return nil
	}
	return ValidateDecisionsLane(cfg.Profile, *cfg.Decisions)
}

// ValidateDecisionsLane checks the decisions lane against the profile it is
// declared under, with the rules the embeddings lane carries: a provider that
// speaks the wire, a dialable endpoint on every profile, and under sovereign a
// local provider on an endpoint the installation controls. The lane reads the
// same text the chat tiers do, so it gets no weaker rule than they get.
//
// Exported because the certification lane builds a lane from its environment
// rather than parsing one, and still has to meet this rule.
func ValidateDecisionsLane(profile Profile, lane DecisionsConfig) error {
	d, known := providerByName(lane.Provider)
	if !known || !d.caps.has(capDecision) {
		return fmt.Errorf("ai: routing config: the decisions lane names %q, which answers no decisions (have: %s)",
			lane.Provider, strings.Join(DecisionProviders(), ", "))
	}
	if strings.TrimSpace(lane.Model) == "" {
		return fmt.Errorf("ai: routing config: the decisions lane names no model")
	}
	if defaulted(lane.BaseURL, d.defaultEndpoint) == "" {
		return fmt.Errorf("ai: routing config: the decisions lane binds %s, which has no default endpoint; set base_url "+
			"to the full decision endpoint URL, e.g. %s or %s", d.name, exampleBrokerDecisionEndpoint, exampleSelfHostedDecisionEndpoint)
	}
	if profile == ProfileSovereign {
		if !d.local && !d.localByEndpoint {
			return fmt.Errorf("ai: routing config: profile sovereign forbids cloud provider %q on the decisions lane", lane.Provider)
		}
		if err := requireSovereignEndpoint("the decisions lane", lane.Provider, lane.BaseURL); err != nil {
			return err
		}
	}
	return requireDialableEndpoint("the decisions lane", lane.Provider, lane.BaseURL)
}

// The two shapes a jev_compatible endpoint takes, named in the refusal that
// asks for one: the broker's decisions endpoint and a self-hosted server.
const (
	exampleBrokerDecisionEndpoint     = "https://openrouter.ai/api/alpha/decisions"
	exampleSelfHostedDecisionEndpoint = "http://127.0.0.1:8767/v1/systemone"
)

// isLocal reports whether the lane's inference stays on infrastructure the
// customer controls: a local adapter always, an endpoint-local one exactly
// when its base_url's host is the customer's own. The local-only rule and the
// routing preview both read this one answer, so the preview cannot promise a
// lane the runtime then skips.
func (lane DecisionsConfig) isLocal() bool {
	d, _ := providerByName(lane.Provider)
	if d.local {
		return true
	}
	if !d.localByEndpoint {
		return false
	}
	host, err := hostOf(lane.BaseURL)
	return err == nil && classifyHost(host) == hostIsLocal
}

// refuseDecisionOnlyProvider refuses a chat binding (a tier, the embeddings
// lane) that names an adapter answering only decisions. It would fail at the
// first call; refused at the write, the operator is told where it belongs.
func refuseDecisionOnlyProvider(label, provider string) error {
	d, known := providerByName(provider)
	if known && !speaksChat(d) {
		return fmt.Errorf("ai: routing config: %s names %q, which answers decisions, not chat; bind it under `decisions:`", label, provider)
	}
	return nil
}

// decisionsResidencyGap refuses a decisions lane under eu_hosted that nothing
// holds to an EU host: the vendor's own API, which promises none, and a broker,
// whose decisions endpoint takes no `only:` pin. eu_hosted is the residency the
// operator chose.
func (cfg RoutingConfig) decisionsResidencyGap() error {
	lane := cfg.Decisions
	if cfg.Profile != ProfileEUHosted || lane == nil {
		return nil
	}
	if providerIsVendorHosted(lane.Provider) {
		return fmt.Errorf("ai: routing config: the decisions lane under profile eu_hosted: %s is its vendor's own API, "+
			"which is not pinned to an EU host; unbind the lane, or declare profile cloud_frontier "+
			"if this installation does not promise EU inference", lane.Provider)
	}
	if !IsOpenRouterHost(lane.BaseURL) {
		return nil
	}
	return fmt.Errorf("ai: routing config: the decisions lane under profile eu_hosted: %s reaches OpenRouter, "+
		"whose decisions endpoint cannot be pinned to an EU host; unbind the lane, or declare profile cloud_frontier "+
		"if this installation does not promise EU inference", lane.Provider)
}
