// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tier→model binding surface (ai-operational-spec §1.4): read what this
// installation is bound to, replace it without a restart.
//
// Thin transport. The ai store owns the RBAC gate, the validation the routing
// file was always held to, and the audit-only write; what this file adds is the
// wire mapping and the human-only refusal.

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type aiRoutingHandlers struct {
	store *ai.RoutingStore
}

func (h aiRoutingHandlers) GetAiRouting(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAiRouting")
		return
	}
	cfg, err := h.store.Get(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeAiRouting(w, r, cfg)
}

// writeAiRouting answers with the document and its revision as the ETag.
func writeAiRouting(w http.ResponseWriter, r *http.Request, cfg ai.RoutingConfig) {
	out, err := toContractAiRouting(cfg)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("ETag", `"`+cfg.Revision()+`"`)
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (h aiRoutingHandlers) ReplaceAiRouting(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "ReplaceAiRouting")
		return
	}
	// Human-only (x-agent-access). An agent never re-points which vendor
	// processes the installation's correspondence, whatever its passport scopes
	// admit. The store re-checks the admin/ops object grant.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.AiRouting
	if !httperr.Decode(w, r, &req) {
		return
	}
	expected, err := routingPrecondition(r.Header)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	next, err := fromContractAiRouting(req, sentRouting(r))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	cfg, err := h.store.ReplaceIfVersion(r.Context(), next, expected)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeAiRouting(w, r, cfg)
}

// GetAiRoutingSchema serves the routing document's JSON Schema, the one the
// editor gate holds to the parser, for the admin screen's field reference.
func (h aiRoutingHandlers) GetAiRoutingSchema(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetAiRoutingSchema")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	schema, err := h.store.RoutingSchema(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/schema+json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(schema); err != nil {
		slog.WarnContext(r.Context(), "writing the routing schema", "err", err)
	}
}

// toContractAiRouting maps a stored binding onto the wire shape.
//
// Tiers is always a map, never nil: an unbound installation answers `{}`, which
// says "nothing is bound", where a null would leave a client guessing whether
// the field was omitted or the read failed.
//
// A lane's base_url and location are its provider's, so a client that
// predates `providers` still sees where each lane goes and writes back a value
// the store recognises as the provider's. Routing goes out as stored: resolving
// it would write the provider's pins and the product default onto every tier
// such a client saves.
func toContractAiRouting(cfg ai.RoutingConfig) (crmcontracts.AiRouting, error) {
	host := func(provider string) string { return cfg.Providers[provider].BaseURL }
	location := func(provider string) string { return cfg.Providers[provider].Location }
	tiers := make(map[string]crmcontracts.AiTierBinding, len(cfg.Tiers))
	for tier, b := range cfg.Tiers {
		routing, err := routingToWire(b.Routing)
		if err != nil {
			return crmcontracts.AiRouting{}, err
		}
		tiers[string(tier)] = crmcontracts.AiTierBinding{
			Provider: b.Provider, Model: b.Model,
			BaseUrl: optionalString(cmp.Or(b.BaseURL, host(b.Provider))), Location: optionalString(cmp.Or(b.Location, location(b.Provider))), Input: optionalStrings(b.Input),
			Routing:       routing,
			ThinkingLevel: optionalEnum[crmcontracts.AiTierBindingThinkingLevel](b.ThinkingLevel),
		}
	}
	embeddingsRouting, err := routingToWire(cfg.Embeddings.Routing)
	if err != nil {
		return crmcontracts.AiRouting{}, err
	}
	return crmcontracts.AiRouting{
		Profile: crmcontracts.AiRoutingProfile(cfg.Profile),
		Tiers:   tiers,
		Embeddings: crmcontracts.AiEmbeddingsBinding{
			Provider: cfg.Embeddings.Provider, Model: cfg.Embeddings.Model,
			BaseUrl:       optionalString(cmp.Or(cfg.Embeddings.BaseURL, host(cfg.Embeddings.Provider))),
			Location:      optionalString(cmp.Or(cfg.Embeddings.Location, location(cfg.Embeddings.Provider))),
			Input:         optionalStrings(cfg.Embeddings.Input),
			Routing:       embeddingsRouting,
			ThinkingLevel: optionalEnum[crmcontracts.AiEmbeddingsBindingThinkingLevel](cfg.Embeddings.ThinkingLevel),
			// Reported as stored rather than as defaulted, so a round-trip of
			// GET → PUT does not silently freeze today's compiled default into
			// the document as though an operator had chosen it.
			Dimensions: optionalInt(cfg.Embeddings.Dimensions),
		},
		Decisions: decisionsToWire(cfg.Decisions, host),
		Providers: providersToWire(cfg.Providers),
	}, nil
}

// decisionsToWire and decisionsFromWire carry the decision lane. The pointer is
// the meaning, as with routing: nil is "no decision model", which sends every
// decision site to its LLM ladder, so neither direction may invent a lane.
func decisionsToWire(d *ai.DecisionsConfig, host func(provider string) string) *crmcontracts.AiDecisionsBinding {
	if d == nil {
		return nil
	}
	return &crmcontracts.AiDecisionsBinding{Provider: d.Provider, Model: d.Model, BaseUrl: optionalString(cmp.Or(d.BaseURL, host(d.Provider)))}
}

func decisionsFromWire(d *crmcontracts.AiDecisionsBinding) *ai.DecisionsConfig {
	if d == nil {
		return nil
	}
	out := &ai.DecisionsConfig{Provider: d.Provider, Model: d.Model}
	if d.BaseUrl != nil {
		out.BaseURL = *d.BaseUrl
	}
	return out
}

// fromContractAiRouting maps a submitted document onto a routing config. It
// validates nothing beyond reading each routing value: the store holds the
// document to the bar the file loader applies, so there is one place a bad
// binding is refused. Every unreadable routing value is refused at once.
func fromContractAiRouting(req crmcontracts.AiRouting, sent map[string]json.RawMessage) (ai.RoutingConfig, error) {
	// The embeddings lane carries routing too: it may narrow which hosts read
	// the text (only, ignore, allow_fallbacks), and the store refuses the rest.
	embeddings := crmcontracts.AiTierBinding{
		Provider: req.Embeddings.Provider, Model: req.Embeddings.Model,
		BaseUrl: req.Embeddings.BaseUrl, Location: req.Embeddings.Location, Input: req.Embeddings.Input,
		Routing: req.Embeddings.Routing,
	}
	// Mapped although this lane refuses it, so a submitted level meets the
	// store's refusal instead of being dropped as though it were never sent.
	if req.Embeddings.ThinkingLevel != nil {
		level := crmcontracts.AiTierBindingThinkingLevel(*req.Embeddings.ThinkingLevel)
		embeddings.ThinkingLevel = &level
	}
	lane, laneErr := tierFromWire(ai.EmbeddingsRoutingPath, embeddings, sent)
	cfg := ai.RoutingConfig{
		Profile:    ai.Profile(req.Profile),
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: lane},
		Decisions:  decisionsFromWire(req.Decisions),
		Providers:  providersFromWire(req.Providers),
	}
	if req.Embeddings.Dimensions != nil {
		cfg.Embeddings.Dimensions = *req.Embeddings.Dimensions
	}
	errs := []error{laneErr}
	if len(req.Tiers) > 0 {
		cfg.Tiers = make(map[ai.Tier]ai.ProviderConfig, len(req.Tiers))
		for name, b := range req.Tiers {
			tier, err := tierFromWire(ai.TierRoutingPath(ai.Tier(name)), b, sent)
			cfg.Tiers[ai.Tier(name)] = tier
			errs = append(errs, err)
		}
	}
	return cfg, ai.JoinRoutingFaults(errs...)
}

func tierFromWire(path string, b crmcontracts.AiTierBinding, sent map[string]json.RawMessage) (ai.ProviderConfig, error) {
	routing, err := routingFromWire(path, b.Routing, sent[path])
	if err != nil {
		return ai.ProviderConfig{}, err
	}
	out := ai.ProviderConfig{Provider: b.Provider, Model: b.Model, Routing: routing}
	if b.BaseUrl != nil {
		out.BaseURL = *b.BaseUrl
	}
	if b.Location != nil {
		out.Location = *b.Location
	}
	if b.Input != nil {
		out.Input = *b.Input
	}
	if b.ThinkingLevel != nil {
		out.ThinkingLevel = string(*b.ThinkingLevel)
	}
	return out, nil
}

// The omitempty helpers exist so an absent value reads as absent rather
// than as a deliberate empty: "no base_url override" and "base_url set to the
// empty string" are the same to a Go zero value and different to an operator.
func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func optionalEnum[E ~string](v string) *E {
	if v == "" {
		return nil
	}
	out := E(v)
	return &out
}

func optionalStrings(v []string) *[]string {
	if len(v) == 0 {
		return nil
	}
	return &v
}

func optionalInt(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func optionalFloat(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ListAvailableModels asks one vendor what it serves, for the form that binds a
// lane to it.
//
// A vendor that cannot be asked is a 200 carrying the reason, not an error: the
// routing form still binds any id a reader types, and turning "your local
// ollama is not running" into a failed request would take the settings page
// down with it. Only the RBAC refusal is an error, because that one is about
// the reader rather than about the vendor.
func (h aiRoutingHandlers) ListAvailableModels(
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	params crmcontracts.ListAvailableModelsParams,
) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "ListAvailableModels")
		return
	}
	// The lane is optional, and an absent one is not a refusal: it only narrows
	// WHICH stored binding supplies the host, and every installation that binds
	// its vendors at one host each has nothing to narrow.
	tier := ""
	if params.Tier != nil {
		tier = *params.Tier
	}
	top := 0
	if params.Top != nil {
		top = *params.Top
	}
	available, err := h.store.ListAvailableModels(r.Context(), ai.AvailableModelsQuery{
		Provider: provider, Tier: tier, Top: top,
		Location: derefString(params.Location), Model: derefString(params.Model),
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractAvailableModels(available))
}

// ListProviderLocations reports where one vendor can process a call, for the
// Location field of a gemini_vertex binding. Like the model list, a vendor
// that cannot be asked is a 200 carrying the reason.
func (h aiRoutingHandlers) ListProviderLocations(w http.ResponseWriter, r *http.Request, provider string) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "ListProviderLocations")
		return
	}
	// Human-only (x-agent-access): the agent gate refuses first, and this is
	// its in-handler twin, as on the binding write.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	found, err := h.store.ListProviderLocations(r.Context(), provider)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractProviderLocations(found))
}

// toContractProviderLocations keeps Locations an array, never null, for the
// reason toContractAvailableModels does.
func toContractProviderLocations(found ai.ProviderLocations) crmcontracts.ProviderLocationList {
	locations := make([]crmcontracts.ProviderLocation, 0, len(found.Locations))
	for _, l := range found.Locations {
		locations = append(locations, crmcontracts.ProviderLocation{
			Id: l.ID, DisplayName: l.DisplayName,
			Jurisdiction: crmcontracts.ProviderLocationJurisdiction(l.Jurisdiction), Resident: l.Resident,
		})
	}
	out := crmcontracts.ProviderLocationList{Provider: found.Provider, Locations: locations}
	if found.Unavailable != ai.AvailabilityOK {
		reason := crmcontracts.ProviderLocationListUnavailable(found.Unavailable)
		out.Unavailable = &reason
	}
	return out
}

// toContractAvailableModels maps one vendor's answer onto the wire shape.
//
// Models is always an array, never nil: a vendor that answered with nothing and
// a read that failed are different states, and the second is what `unavailable`
// is for — a null here would leave a client guessing which it had.
func toContractAvailableModels(a ai.AvailableModels) crmcontracts.AvailableModelList {
	models := make([]crmcontracts.AvailableModel, 0, len(a.Models))
	for _, m := range a.Models {
		models = append(models, crmcontracts.AvailableModel{
			Id:            m.ID,
			DisplayName:   optionalString(m.DisplayName),
			Lane:          availableModelLane(m.Lane),
			ContextLength: m.ContextLength,
			InputPerMtok:  m.InputPerMtok,
			OutputPerMtok: m.OutputPerMtok,
			RankScore:     m.RankScore,
		})
	}
	out := crmcontracts.AvailableModelList{Provider: a.Provider, Models: models, RankedBy: optionalString(a.RankedBy)}
	if a.Complete {
		out.Complete = &a.Complete
	}
	if a.Unavailable != ai.AvailabilityOK {
		reason := crmcontracts.AvailableModelListUnavailable(a.Unavailable)
		out.Unavailable = &reason
	}
	return out
}

// availableModelLane carries a STATED lane and nothing else. An empty lane is
// the vendor declining to say, which the wire spells as an absent field —
// defaulting it to chat would claim the vendor said something it did not, and
// an embedder offered on a chat tier cannot serve a call.
func availableModelLane(lane string) *crmcontracts.AvailableModelLane {
	if lane == "" {
		return nil
	}
	out := crmcontracts.AvailableModelLane(lane)
	return &out
}

// TestAiProviderKey asks one vendor whether the stored credential works.
//
// Human-only, like every other route that reaches this installation's
// credentials. A vendor that refused or could not be asked is a 200 carrying
// the reason: the test ran and that is its result.
func (h aiRoutingHandlers) TestAiProviderKey(w http.ResponseWriter, r *http.Request, provider string) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "TestAiProviderKey")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	tested, err := h.store.TestProviderKey(r.Context(), provider)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractKeyTest(tested))
}

// toContractKeyTest maps a key test onto the wire. `reason` is present only on
// a failure, and `model_count` only on a pass that listed models — a vendor
// tested at a key endpoint passes with no count rather than with a zero.
func toContractKeyTest(t ai.KeyTest) crmcontracts.AiProviderKeyTestResult {
	out := crmcontracts.AiProviderKeyTestResult{Provider: t.Provider, Ok: t.OK}
	if t.OK {
		if t.Counted {
			count := t.ModelCount
			out.ModelCount = &count
		}
		confirmed := !t.Unconfirmed
		out.KeyConfirmed = &confirmed
		return out
	}
	reason := crmcontracts.AiProviderKeyTestResultReason(t.Reason)
	out.Reason = &reason
	return out
}

// The routing resource always exists, including its unconfigured default.
// Absence and * permit replacement; an explicitly empty tag must not unpin it.
func routingPrecondition(header http.Header) (string, error) {
	_, present := header["If-Match"]
	value := strings.TrimSpace(header.Get("If-Match"))
	if !present || value == "*" {
		return "", nil
	}
	value = strings.Trim(value, `"`)
	if value == "" {
		return "", apperrors.ErrVersionSkew
	}
	return value, nil
}
