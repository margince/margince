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
// speaks one wire, so the recipe is the registry row — its path, its default
// endpoint and whose key it sends — and no per-provider switch.
func selectDeciderOn(lane DecisionsConfig, keys config.Lookup, httpc *http.Client) (*decisionClient, error) {
	d, known := providerByName(lane.Provider)
	if !known || !d.caps.has(capDecision) {
		return nil, fmt.Errorf("ai: unknown decision provider %q (have: %s)", lane.Provider, strings.Join(DecisionProviders(), ", "))
	}
	client := &decisionClient{http: httpc}
	if d.keyOwner != "" {
		client.apiKey = cloudKey(d.keyOwner, keys)
		if client.apiKey == "" {
			return nil, byokKeyRequired(d.keyOwner)
		}
	}
	base := defaulted(lane.BaseURL, d.defaultBaseURL)
	if base == "" {
		return nil, fmt.Errorf("%w: %s (the endpoint root, e.g. https://openrouter.ai/api)", errNoBaseURL, lane.Provider)
	}
	client.url = strings.TrimRight(base, "/") + d.decisionPath
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
