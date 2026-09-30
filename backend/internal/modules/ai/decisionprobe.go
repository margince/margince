// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// What a decision endpoint serves, and whether it takes the stored key, asked
// without spending a decision.
//
// The Jev wire promises ONE route — the decision POST — so "compatible" says
// nothing about a list. Each host is therefore asked in the way it actually
// answers:
//
//   - TypeSafe's own API publishes GET /v1/models beside /v1/systemone, behind
//     the key: one call lists the models and proves the key.
//   - OpenRouter's catalogue is public, so listing proves nothing about a key;
//     GET /api/v1/key is its authenticated no-cost read, and its catalogue —
//     asked for every output modality, or Jev is left out — is the list.
//   - Any other server is asked the decision POST with an empty body: a 401/403
//     is a refused key, and a 400 says only that it answered without refusing —
//     an unconfirmed pass. No model is named, so nothing is billed.
//
// A lane the profile would refuse to bind is not dialled either: testing a key
// for a decision endpoint eu_hosted forbids answers profile_forbids, the same
// verdict the routing validator gives.

// decisionHost is which of those three a decision endpoint is.
type decisionHost int

const (
	decisionHostWire decisionHost = iota
	decisionHostTypeSafe
	decisionHostOpenRouter
)

// decisionHostFor classifies an endpoint. `jev_compatible` on OpenRouter is
// recognised by host, by the same predicate its routing preferences use; `jev`
// is TypeSafe's API by contract, so whatever host it is pointed at is read as
// serving TypeSafe's routes, and its key goes to no other route.
func decisionHostFor(provider, endpoint string) decisionHost {
	if provider == providerJevCompatible && IsOpenRouterHost(endpoint) {
		return decisionHostOpenRouter
	}
	if provider == providerJev {
		return decisionHostTypeSafe
	}
	return decisionHostWire
}

// siblingURL is the route `last` beside the endpoint, replacing its final
// segment: TypeSafe serves /v1/models beside /v1/systemone, and a proxy that
// mounts that API under a prefix keeps the prefix.
func siblingURL(endpoint, last string) (string, error) {
	origin, path, err := splitEndpoint(endpoint)
	if err != nil {
		return "", err
	}
	path = strings.TrimSuffix(path, "/")
	if !strings.Contains(path, "/") {
		path = "/"
	}
	return origin + path[:strings.LastIndex(path, "/")+1] + last, nil
}

// originURL is `path` at the endpoint's own scheme and host.
func originURL(endpoint, path string) (string, error) {
	origin, _, err := splitEndpoint(endpoint)
	if err != nil {
		return "", err
	}
	return origin + path, nil
}

func splitEndpoint(endpoint string) (origin, path string, err error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return "", "", fmt.Errorf("ai: decision endpoint %q is not a URL", endpoint)
	}
	return parsed.Scheme + "://" + parsed.Host, parsed.Path, nil
}

func (c *decisionClient) authorize(r *http.Request) {
	if c.apiKey != "" {
		r.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// typeSafeModels lists TypeSafe's served names. The API lists aliases
// (`jev-latest`); a versioned id it does not list is still accepted, which is
// why the picker keeps the field free-text.
func (c *decisionClient) typeSafeModels(ctx context.Context) ([]model.Info, error) {
	endpoint, err := siblingURL(c.url, "models")
	if err != nil {
		return nil, err
	}
	raw, err := getListBody(ctx, c.http, providerJev, endpoint, c.authorize)
	if err != nil {
		return nil, err
	}
	// A pointer, so a 200 that is not the list — a proxy's `{}` — is told apart
	// from a list that is empty, and is not read as a confirmed key.
	var out struct {
		Models *[]struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai: jev: decode model list: %w", err)
	}
	if out.Models == nil {
		return nil, fmt.Errorf("ai: jev: the answer carries no model list")
	}
	models := make([]model.Info, 0, len(*out.Models))
	for _, m := range *out.Models {
		if len(models) == modelListLimit {
			break
		}
		models = append(models, model.Info{ID: m.Name, Lane: model.LaneDecisions})
	}
	return models, nil
}

// openRouterKeyCheck reads OpenRouter's own record of the key: authenticated,
// and free.
func (c *decisionClient) openRouterKeyCheck(ctx context.Context) error {
	endpoint, err := originURL(c.url, "/api/v1/key")
	if err != nil {
		return err
	}
	_, err = getListBody(ctx, c.http, openRouterProvider, endpoint, c.authorize)
	return err
}

// openRouterDecisionModels is OpenRouter's catalogue narrowed to TypeSafe's
// models. The default listing omits every model whose output is not text,
// which is every Jev but the router, so all modalities are asked for.
func (c *decisionClient) openRouterDecisionModels(ctx context.Context) ([]model.Info, error) {
	endpoint, err := originURL(c.url, "/api/v1/models?output_modalities=all")
	if err != nil {
		return nil, err
	}
	raw, err := getListBody(ctx, c.http, openRouterProvider, endpoint, c.authorize)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai: openrouter: decode model list: %w", err)
	}
	var models []model.Info
	for _, m := range out.Data {
		if strings.HasPrefix(strings.TrimPrefix(m.ID, "~"), "typesafe/") {
			models = append(models, model.Info{ID: m.ID, DisplayName: m.Name, Lane: model.LaneDecisions})
		}
	}
	return models, nil
}

// wireProbe sends the decision POST an empty body. A 401/403 is a refused key;
// a 400 or 422 means the server answered and did not refuse it — which is all
// the wire can prove, since a server may validate the body before the key, and
// the pass is reported as unconfirmed. Nothing was asked, so nothing is billed.
func (c *decisionClient) wireProbe(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return fmt.Errorf("ai: decision probe: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	resp, err := noRedirect(c.http).Do(req)
	if err != nil {
		return fmt.Errorf("ai: decision probe: %w", err)
	}
	//craft:ignore swallowed-errors best-effort close of a body this probe never reads
	defer func() { _ = resp.Body.Close() }()
	// 200 is not a pass: a Jev server cannot answer an empty request, so a
	// 200 is something else at that address — a landing page, a catch-all —
	// that checked no key at all.
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil
	default:
		return &listStatusError{vendor: "decision", status: resp.StatusCode}
	}
}

// decisionModels is what the picker offers for a decision endpoint, or
// ok=false where the host publishes no list.
func (c *decisionClient) decisionModels(ctx context.Context, provider string) ([]model.Info, bool, error) {
	switch decisionHostFor(provider, c.url) {
	case decisionHostTypeSafe:
		models, err := c.typeSafeModels(ctx)
		return models, true, err
	case decisionHostOpenRouter:
		models, err := c.openRouterDecisionModels(ctx)
		return models, true, err
	default:
		return nil, false, nil
	}
}

// probeKey tests the stored key against a decision endpoint. counted is
// whether the answer came with a model list whose length means something.
func (c *decisionClient) probeKey(ctx context.Context, provider string) (count int, counted bool, err error) {
	switch decisionHostFor(provider, c.url) {
	case decisionHostTypeSafe:
		models, err := c.typeSafeModels(ctx)
		return len(models), err == nil, err
	case decisionHostOpenRouter:
		return 0, false, c.openRouterKeyCheck(ctx)
	default:
		return 0, false, c.wireProbe(ctx)
	}
}

// decisionLaneForbidden is whether the profile refuses this lane, read by the
// routing validator's own residency rule rather than a second copy of it.
func decisionLaneForbidden(profile Profile, lane DecisionsConfig) bool {
	return RoutingConfig{Profile: profile, Decisions: &lane}.decisionsResidencyGap() != nil
}

// boundDecisionLane is the decision binding a test or a list uses for
// provider: the stored lane when it names this provider, otherwise the
// adapter's default endpoint — which `jev_compatible`, having none, refuses.
func boundDecisionLane(cfg RoutingConfig, provider string) DecisionsConfig {
	if cfg.Decisions != nil && cfg.Decisions.Provider == provider {
		return *cfg.Decisions
	}
	return DecisionsConfig{Provider: provider}
}
