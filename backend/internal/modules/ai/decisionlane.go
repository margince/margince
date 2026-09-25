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
	if profile == ProfileSovereign {
		if !d.local {
			return fmt.Errorf("ai: routing config: profile sovereign forbids cloud provider %q on the decisions lane", lane.Provider)
		}
		if err := requireSovereignEndpoint("the decisions lane", lane.Provider, lane.BaseURL); err != nil {
			return err
		}
	}
	if err := requireDialableEndpoint("the decisions lane", lane.Provider, lane.BaseURL); err != nil {
		return err
	}
	// The provider word promises OpenRouter's decisions endpoint, and the
	// adapter sends the OpenRouter key: a base_url anywhere else would carry
	// that key to a host that never issued it.
	if d.keyOwner == providerOpenAICompatible && !IsOpenRouterHost(lane.BaseURL) {
		return fmt.Errorf("ai: routing config: the decisions lane binds %s, which is OpenRouter's decisions endpoint; "+
			"set base_url to an OpenRouter host, e.g. https://openrouter.ai/api", d.name)
	}
	return nil
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

// decisionsResidencyGap refuses a broker decisions lane under eu_hosted. The
// decisions endpoint takes no `only:` pin, so nothing can hold the broker to
// an EU host, and eu_hosted is the residency the operator chose.
func (cfg RoutingConfig) decisionsResidencyGap() error {
	if cfg.Profile != ProfileEUHosted || cfg.Decisions == nil || !IsOpenRouterHost(cfg.Decisions.BaseURL) {
		return nil
	}
	return fmt.Errorf("ai: routing config: the decisions lane under profile eu_hosted: %s reaches OpenRouter, "+
		"whose decisions endpoint cannot be pinned to an EU host; unbind the lane, or declare profile cloud_frontier "+
		"if this installation does not promise EU inference", cfg.Decisions.Provider)
}
