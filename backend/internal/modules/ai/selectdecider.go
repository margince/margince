// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// decisionLane is a bound decisions lane: the client that asks it and the
// binding it was built from, which the trace and the rate lookup name.
type decisionLane struct {
	client decision.Client
	meta   routeMeta
}

// selectDecider builds the decisions lane's client, guarded by the same
// egress rule a chat adapter of that provider gets.
//
// Held by: TestOnlyTheSelectorsBuildAnOutboundClient (backend/internal/modules/ai/outboundegress_test.go)
func selectDecider(lane DecisionsConfig, keys config.Lookup) (*decisionClient, error) {
	return selectDeciderOn(lane, keys, newOutboundClient(lane.Provider))
}

// selectDeciderOn is selectDecider with the transport supplied, the seam a
// test binds the client to an httptest server through. Every decision adapter
// speaks one wire, so the recipe is the registry row — its default endpoint
// and its key — and no per-provider switch. The binding's base_url is the full
// endpoint: nothing is appended to it.
func selectDeciderOn(lane DecisionsConfig, keys config.Lookup, httpc *http.Client) (*decisionClient, error) {
	d, known := providerByName(lane.Provider)
	if !known || !d.caps.has(capDecision) {
		return nil, fmt.Errorf("ai: unknown decision provider %q (have: %s)", lane.Provider, strings.Join(DecisionProviders(), ", "))
	}
	client := &decisionClient{http: httpc, url: defaulted(lane.BaseURL, d.defaultEndpoint)}
	if d.keyEnv != "" {
		client.apiKey = cloudKey(d.name, keys)
		if client.apiKey == "" && !d.keyOptional {
			return nil, byokKeyRequired(d.name)
		}
	}
	if client.url == "" {
		return nil, fmt.Errorf("%w: %s (the full decision endpoint, e.g. %s)", errNoBaseURL, lane.Provider, exampleBrokerDecisionEndpoint)
	}
	return client, nil
}

// buildDecisionLane is the bound lane, or nil when the config binds none — the
// state in which every call is exactly the ladder's.
func (cfg RoutingConfig) buildDecisionLane() (*decisionLane, error) {
	lane := cfg.Decisions
	if lane == nil {
		return nil, nil //nolint:nilnil // an unbound lane IS the answer for a config that binds none, not a missing one
	}
	client, err := selectDecider(*lane, cfg.keys)
	if err != nil {
		return nil, fmt.Errorf("ai: decisions lane: %w", err)
	}
	return &decisionLane{client: client, meta: lane.routeMeta()}, nil
}

// routeMeta is the lane's identity as every trace row and rate lookup names it.
func (lane DecisionsConfig) routeMeta() routeMeta {
	return routeMeta{provider: lane.Provider, model: lane.Model, baseURL: lane.BaseURL}
}
