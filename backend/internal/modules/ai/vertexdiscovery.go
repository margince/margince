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
const vertexMetadataLocation = "global"

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
		ProviderConfig{Provider: providerGeminiVertex, Location: vertexMetadataLocation}, s.resolvedKeys(ctx))
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
	"eu":     "EU (multi-region)",
	"us":     "US (multi-region)",
	"global": "Global",
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
		var page struct {
			Locations []struct {
				LocationID  string `json:"locationId"`  //nolint:tagliatelle // Google's wire format (camelCase)
				DisplayName string `json:"displayName"` //nolint:tagliatelle // Google's wire format (camelCase)
			} `json:"locations"`
			NextPageToken string `json:"nextPageToken"` //nolint:tagliatelle // Google's wire format (camelCase)
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("ai: gemini_vertex: decode location list: %w", err)
		}
		for _, l := range page.Locations {
			found[l.LocationID] = l.DisplayName
		}
		if page.NextPageToken == "" || len(found) >= modelListLimit {
			return found, nil
		}
		pageToken = page.NextPageToken
	}
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
	case errors.Is(err, errNoProviderKey):
		out.Unavailable = AvailabilityNoKey
	default:
		out.Unavailable = AvailabilityUnreachable
	}
	return out
}

// probeBinding builds the binding's client and asks it about one model.
func (s *RoutingStore) probeBinding(ctx context.Context, bound ProviderConfig, id, lane string) error {
	client, err := s.selectBrain.build(bound, s.resolvedKeys(ctx))
	if err != nil {
		return err
	}
	gemini, ok := client.(*geminiClient)
	if !ok {
		return fmt.Errorf("ai: %s has no model probe", bound.Provider)
	}
	asked, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	return gemini.probeModel(asked, id, lane)
}

// probeVertexBindings asks each gemini_vertex binding's location for its
// model before the binding is stored: a model the location does not serve
// would otherwise be found by the first real call.
func (s *RoutingStore) probeVertexBindings(ctx context.Context, cfg RoutingConfig) error {
	for _, tier := range sortedTiers(cfg.Tiers) {
		if binding := cfg.Tiers[tier]; binding.Provider == providerGeminiVertex {
			if err := s.refuseUnserved(ctx, "tier "+string(tier), binding, binding.Model, model.LaneChat); err != nil {
				return err
			}
		}
	}
	if embeddings := cfg.Embeddings.ProviderConfig; embeddings.Provider == providerGeminiVertex {
		return s.refuseUnserved(ctx, "embeddings", embeddings, defaulted(embeddings.Model, geminiEmbedModel), model.LaneEmbeddings)
	}
	return nil
}

func (s *RoutingStore) refuseUnserved(ctx context.Context, label string, binding ProviderConfig, id, lane string) error {
	err := s.probeBinding(ctx, binding, id, lane)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errModelNotFound):
		return fmt.Errorf("ai: routing config: %s: gemini_vertex does not serve model %q in location %q", label, id, binding.Location)
	case errors.Is(err, errNoProviderKey):
		return fmt.Errorf("ai: routing config: %s: gemini_vertex holds no service-account key, so model %q in location %q cannot be checked — add the key first", label, id, binding.Location)
	default:
		return fmt.Errorf("ai: routing config: %s: Google could not be asked whether location %q serves model %q — check the service-account key and try again", label, binding.Location, id)
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
