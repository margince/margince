// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// What a gemini_vertex binding can be pointed at, for the screen that binds
// one: the locations Google offers, and whether one location serves a model.
// Both run through a client SelectBrain built, so the token exchange and the
// calls share the one guarded transport every adapter uses.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// vertexMetadataLocation builds a client for a call whose host is fixed
// (the locations list, the token exchange): selectVertex needs a location,
// and global is the one that names no jurisdiction.
const vertexMetadataLocation = vertexGlobal

// vertexProbeWord is the whole input a probe sends.
const vertexProbeWord = "ping"

// ProviderLocation is one place a vendor can process a call. Jurisdiction
// and Resident are this build's policy (residency.go), never the vendor's.
type ProviderLocation struct {
	ID, DisplayName string
	Jurisdiction    string
	Resident        bool
}

// ProviderLocations is one vendor's answer. Unavailable is set exactly when
// Locations is empty, and names why.
type ProviderLocations struct {
	Provider    string
	Locations   []ProviderLocation
	Unavailable ModelAvailability
}

// ListProviderLocations asks Google which locations the stored key's project
// can reach. Answered under every profile: the list is metadata on the global
// host, and under eu_resident the non-resident options are still shown, marked
// so, because the screen explains the refusal rather than hiding the choice.
func (s *RoutingStore) ListProviderLocations(ctx context.Context, provider string) (ProviderLocations, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return ProviderLocations{}, err
	}
	out := ProviderLocations{Provider: provider}
	if provider != providerGeminiVertex {
		out.Unavailable = AvailabilityNotPublished
		return out, nil
	}
	client, err := s.selectBrain.build(
		ProviderConfig{Provider: providerGeminiVertex, Location: vertexMetadataLocation}, s.resolvedKeys(ctx),
	)
	if err != nil {
		out.Unavailable = unavailableFor(err)
		return out, nil
	}
	vertex, ok := vertexOf(client)
	if !ok {
		out.Unavailable = AvailabilityNotPublished
		return out, nil
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	fetched, err := vertex.listLocations(asked)
	if err != nil {
		out.Unavailable = AvailabilityUnreachable
		return out, nil
	}
	out.Locations = placedLocations(fetched)
	return out, nil
}

// vertexMultiRegions are offered whether or not Google's list names them:
// they are addressable on every project, and eu is the resident default.
var vertexMultiRegions = map[string]string{
	"eu":         "EU (multi-region)",
	"us":         "US (multi-region)",
	vertexGlobal: "Global",
}

// placedLocations keeps the ids a binding could name, adds the multi-regions
// Google left out, and stamps each with this build's residency policy.
func placedLocations(fetched map[string]string) []ProviderLocation {
	named := make(map[string]string, len(fetched)+len(vertexMultiRegions))
	for id, display := range fetched {
		if vertexLocationShape.MatchString(id) {
			named[id] = display
		}
	}
	for id, display := range vertexMultiRegions {
		if named[id] == "" {
			named[id] = display
		}
	}
	out := make([]ProviderLocation, 0, len(named))
	for _, id := range slices.Sorted(maps.Keys(named)) {
		out = append(out, ProviderLocation{
			ID: id, DisplayName: named[id],
			Jurisdiction: locationJurisdiction(id), Resident: euResidentLocations[id],
		})
	}
	return out
}

// vertexClient is a geminiClient known to speak to Vertex AI, which is what
// the location list and the credential check need beyond model.Client.
type vertexClient struct {
	*geminiClient
	vertex vertexTransport
}

func vertexOf(client model.Client) (vertexClient, bool) {
	gemini, ok := client.(*geminiClient)
	if !ok {
		return vertexClient{}, false
	}
	transport, ok := gemini.transport.(vertexTransport)
	return vertexClient{geminiClient: gemini, vertex: transport}, ok
}

// listLocations follows GET /v1/projects/{p}/locations to its last page,
// returning id → display name.
func (c vertexClient) listLocations(ctx context.Context) (map[string]string, error) {
	found := map[string]string{}
	pageToken := ""
	for {
		endpoint := vertexHost(vertexMetadataLocation) + "/v1/projects/" + c.vertex.projectID + "/locations?pageSize=100"
		if pageToken != "" {
			endpoint += "&pageToken=" + url.QueryEscape(pageToken)
		}
		raw, err := getListBody(ctx, c.http, providerGeminiVertex, endpoint, func(r *http.Request) error {
			return c.vertex.authorize(ctx, r)
		})
		if err != nil {
			return nil, err
		}
		next, err := readLocationPage(raw, found)
		if err != nil {
			return nil, err
		}
		if next == "" || len(found) >= modelListLimit {
			return found, nil
		}
		pageToken = next
	}
}

// readLocationPage adds one page of Google's location list to found and
// returns the next page's token.
func readLocationPage(raw []byte, found map[string]string) (string, error) {
	var page struct {
		Locations []struct {
			LocationID  string `json:"locationId"`  //nolint:tagliatelle // Google's wire format (camelCase)
			DisplayName string `json:"displayName"` //nolint:tagliatelle // Google's wire format (camelCase)
		} `json:"locations"`
		NextPageToken string `json:"nextPageToken"` //nolint:tagliatelle // Google's wire format (camelCase)
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return "", fmt.Errorf("ai: gemini_vertex: decode location list: %w", err)
	}
	for _, l := range page.Locations {
		found[l.LocationID] = l.DisplayName
	}
	return page.NextPageToken, nil
}

// verifyCredential mints one access token, which is the whole of what a key
// file can prove before a model is called with it.
func (c vertexClient) verifyCredential(ctx context.Context) error {
	_, err := c.vertex.tokens.accessToken(ctx)
	return err
}

// probeModel asks this client's location whether it serves one model, with
// the cheapest call that names the model: countTokens for chat, one
// embedContent for the embeddings lane. errModelNotFound means it does not.
func (c *geminiClient) probeModel(ctx context.Context, id, lane string) error {
	if lane == model.LaneEmbeddings {
		_, err := c.Embed(ctx, model.EmbedRequest{Model: id, Inputs: []string{vertexProbeWord}})
		return err
	}
	wire := struct {
		Contents []geminiContent `json:"contents"`
	}{Contents: []geminiContent{{Role: roleUser, Parts: []geminiPart{{Text: vertexProbeWord}}}}}
	payload, _, err := sendablePayload(ctx, wire, nil)
	if err != nil {
		return err
	}
	body, err := c.post(ctx, c.transport.modelURL(id, "countTokens"), payload)
	if err != nil {
		return err
	}
	return body.Close()
}

// probeLane is the lane a probe asks for, read off the lane being edited.
func probeLane(tier string) string {
	if tier == string(LaneEmbeddings) {
		return model.LaneEmbeddings
	}
	return model.LaneChat
}

// probeAvailability answers a `model` query: that model alone when the
// location serves it, or why not.
func (s *RoutingStore) probeAvailability(ctx context.Context, bound ProviderConfig, q AvailableModelsQuery) AvailableModels {
	out := AvailableModels{Provider: q.Provider}
	if q.Provider != providerGeminiVertex {
		out.Unavailable = AvailabilityNotPublished
		return out
	}
	lane := probeLane(q.Tier)
	switch err := s.probeBinding(ctx, bound, q.Model, lane); {
	case err == nil:
		out.Models = []AvailableModel{{Info: model.Info{ID: q.Model, Lane: lane}}}
	case errors.Is(err, errModelNotFound):
		out.Unavailable = AvailabilityNoEndpoint
	case errors.Is(err, errNoProviderKey), errors.Is(err, errInvalidServiceAccount):
		out.Unavailable = AvailabilityNoKey
	default:
		out.Unavailable = AvailabilityUnreachable
	}
	return out
}

// probeBinding builds the binding's client and asks it about one model.
func (s *RoutingStore) probeBinding(ctx context.Context, bound ProviderConfig, id, lane string) error {
	gemini, err := s.vertexProbeClient(ctx, bound.Location)
	if err != nil {
		return err
	}
	return probeOnce(ctx, gemini, vertexProbe{location: bound.Location, model: id, lane: lane})
}

// vertexProbeClient is built per request, so each discovery click mints its
// own token: a source kept across requests would outlive a replaced key.
func (s *RoutingStore) vertexProbeClient(ctx context.Context, location string) (*geminiClient, error) {
	client, err := s.selectBrain.build(ProviderConfig{Provider: providerGeminiVertex, Location: location}, s.resolvedKeys(ctx))
	if err != nil {
		return nil, err
	}
	gemini, ok := client.(*geminiClient)
	if !ok {
		return nil, fmt.Errorf("ai: %s has no model probe", providerGeminiVertex)
	}
	return gemini, nil
}

// relocated is this client addressed to another location. It shares the
// token source, so asking several locations spends one token exchange.
func (c *geminiClient) relocated(location string) *geminiClient {
	vertex, ok := c.transport.(vertexTransport)
	if !ok {
		return c
	}
	vertex.host, vertex.location = vertexHost(location), location
	moved := *c
	moved.transport = vertex
	return &moved
}

// vertexProbe is one question a save can ask Google: does this location
// serve this model on this lane.
type vertexProbe struct {
	location, model, lane string
}

type labelledProbe struct {
	vertexProbe
	label string
}

// vertexProbesOf lists the distinct questions a config's gemini_vertex
// bindings raise, each under the first lane that raised it.
func vertexProbesOf(cfg RoutingConfig) []labelledProbe {
	var out []labelledProbe
	seen := map[vertexProbe]bool{}
	add := func(label string, binding ProviderConfig, id, lane string) {
		probe := vertexProbe{location: binding.Location, model: id, lane: lane}
		if binding.Provider == providerGeminiVertex && !seen[probe] {
			seen[probe] = true
			out = append(out, labelledProbe{vertexProbe: probe, label: label})
		}
	}
	for _, tier := range sortedTiers(cfg.Tiers) {
		binding := cfg.Tiers[tier]
		add("tier "+string(tier), binding, binding.Model, model.LaneChat)
	}
	embeddings := cfg.Embeddings.ProviderConfig
	add("embeddings", embeddings, defaulted(embeddings.Model, geminiEmbedModel), model.LaneEmbeddings)
	return out
}

// probeVertexBindings asks Google, before the binding is stored, about each
// gemini_vertex binding the save introduces: a model its location does not
// serve would otherwise be found by the first real call. Only what the save
// introduces is asked, so an unrelated edit asks nothing.
//
// Only a definite answer refuses the save. Google failing to answer is
// logged and the save admitted without asking further, because an outage
// there must not freeze, or stall, every routing edit here.
func (s *RoutingStore) probeVertexBindings(ctx context.Context, stored, next RoutingConfig) error {
	asked := map[vertexProbe]bool{}
	for _, p := range vertexProbesOf(stored) {
		asked[p.vertexProbe] = true
	}
	var client *geminiClient
	for _, p := range vertexProbesOf(next) {
		if asked[p.vertexProbe] {
			continue
		}
		var err error
		if client == nil {
			client, err = s.vertexProbeClient(ctx, p.location)
		}
		if err == nil {
			err = probeOnce(ctx, client.relocated(p.location), p.vertexProbe)
		}
		if refusal := refuseUnserved(p, err); refusal != nil {
			return refusal
		}
		if err != nil {
			s.logger().WarnContext(ctx, "ai: routing saved with a gemini_vertex model unchecked: Google did not answer the probe",
				"lane", p.label, "location", p.location, "model", p.model, "error", err.Error())
			return nil
		}
	}
	return nil
}

func probeOnce(ctx context.Context, client *geminiClient, p vertexProbe) error {
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	return client.probeModel(asked, p.model, p.lane)
}

// refuseUnserved is the save's refusal for an answer that settles the
// question, and nil for one that does not.
func refuseUnserved(p labelledProbe, err error) error {
	switch {
	case errors.Is(err, errModelNotFound):
		return fmt.Errorf("ai: routing config: %s: gemini_vertex does not serve model %q in location %q", p.label, p.model, p.location)
	case errors.Is(err, errNoProviderKey):
		return fmt.Errorf("ai: routing config: %s: gemini_vertex holds no service-account key, so model %q in location %q cannot be checked — add the key first", p.label, p.model, p.location)
	case errors.Is(err, errInvalidServiceAccount):
		return fmt.Errorf("ai: routing config: %s: the stored gemini_vertex service-account key is not usable (%w) — replace it under Provider keys", p.label, err)
	default:
		return nil
	}
}

// brainSelector turns a binding into a client. The zero value is SelectBrain;
// an in-package test supplies one whose transport reaches an httptest server.
type brainSelector func(ProviderConfig, config.Lookup) (model.Client, error)

//nolint:ireturn // SelectBrain's own return shape
func (b brainSelector) build(cfg ProviderConfig, keys config.Lookup) (model.Client, error) {
	if b == nil {
		return SelectBrain(cfg, keys)
	}
	return b(cfg, keys)
}
