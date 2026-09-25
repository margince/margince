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
	w.Header().Set("ETag", `"`+cfg.Revision()+`"`)
	httperr.WriteJSON(w, http.StatusOK, toContractAiRouting(cfg))
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
	cfg, err := h.store.ReplaceIfVersion(r.Context(), fromContractAiRouting(req), expected)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("ETag", `"`+cfg.Revision()+`"`)
	httperr.WriteJSON(w, http.StatusOK, toContractAiRouting(cfg))
}

// toContractAiRouting maps a stored binding onto the wire shape.
//
// Tiers is always a map, never nil: an unbound installation answers `{}`, which
// says "nothing is bound", where a null would leave a client guessing whether
// the field was omitted or the read failed.
func toContractAiRouting(cfg ai.RoutingConfig) crmcontracts.AiRouting {
	tiers := make(map[string]crmcontracts.AiTierBinding, len(cfg.Tiers))
	for tier, b := range cfg.Tiers {
		tiers[string(tier)] = crmcontracts.AiTierBinding{
			Provider: b.Provider, Model: b.Model,
			BaseUrl: optionalString(b.BaseURL), Input: optionalStrings(b.Input),
			Routing: routingToWire(b.Routing),
		}
	}
	return crmcontracts.AiRouting{
		Profile: crmcontracts.AiRoutingProfile(cfg.Profile),
		Tiers:   tiers,
		Embeddings: crmcontracts.AiEmbeddingsBinding{
			Provider: cfg.Embeddings.Provider, Model: cfg.Embeddings.Model,
			BaseUrl: optionalString(cfg.Embeddings.BaseURL),
			Input:   optionalStrings(cfg.Embeddings.Input),
			Routing: routingToWire(cfg.Embeddings.Routing),
			// Reported as stored rather than as defaulted, so a round-trip of
			// GET → PUT does not silently freeze today's compiled default into
			// the document as though an operator had chosen it.
			Dimensions: optionalInt(cfg.Embeddings.Dimensions),
		},
	}
}

// fromContractAiRouting maps a submitted document onto a routing config. It
// validates nothing: the store holds it to the same bar the file loader
// applies, so there is exactly one place a bad binding is refused.
func fromContractAiRouting(req crmcontracts.AiRouting) ai.RoutingConfig {
	// The embeddings lane carries routing too: it may narrow which hosts read
	// the text (only, ignore, allow_fallbacks), and the store refuses the rest.
	embeddings := crmcontracts.AiTierBinding{
		Provider: req.Embeddings.Provider, Model: req.Embeddings.Model,
		BaseUrl: req.Embeddings.BaseUrl, Input: req.Embeddings.Input, Routing: req.Embeddings.Routing,
	}
	cfg := ai.RoutingConfig{
		Profile:    ai.Profile(req.Profile),
		Embeddings: ai.EmbeddingsConfig{ProviderConfig: tierFromWire(embeddings)},
	}
	if req.Embeddings.Dimensions != nil {
		cfg.Embeddings.Dimensions = *req.Embeddings.Dimensions
	}
	if len(req.Tiers) > 0 {
		cfg.Tiers = make(map[ai.Tier]ai.ProviderConfig, len(req.Tiers))
		for name, b := range req.Tiers {
			cfg.Tiers[ai.Tier(name)] = tierFromWire(b)
		}
	}
	return cfg
}

func tierFromWire(b crmcontracts.AiTierBinding) ai.ProviderConfig {
	out := ai.ProviderConfig{Provider: b.Provider, Model: b.Model, Routing: routingFromWire(b.Routing)}
	if b.BaseUrl != nil {
		out.BaseURL = *b.BaseUrl
	}
	if b.Input != nil {
		out.Input = *b.Input
	}
	return out
}

// routingToWire and routingFromWire carry a binding's broker preferences. The
// pointer is the meaning: nil is "the product default" and an empty struct is
// "no preferences", so neither direction may turn one into the other.
func routingToWire(r *ai.OpenRouterRouting) *crmcontracts.AiOpenRouterRouting {
	if r == nil {
		return nil
	}
	return &crmcontracts.AiOpenRouterRouting{
		Only: optionalStrings(r.Only), Ignore: optionalStrings(r.Ignore),
		Quantizations: optionalStrings(r.Quantizations), Sort: optionalString(r.Sort),
		RequireParameters: r.RequireParameters, AllowFallbacks: r.AllowFallbacks,
		PreferredMaxLatencyP90: optionalFloat(r.PreferredMaxLatencyP90),
		ReasoningEffort:        optionalString(r.ReasoningEffort),
	}
}

func routingFromWire(r *crmcontracts.AiOpenRouterRouting) *ai.OpenRouterRouting {
	if r == nil {
		return nil
	}
	out := &ai.OpenRouterRouting{RequireParameters: r.RequireParameters, AllowFallbacks: r.AllowFallbacks}
	if r.Only != nil {
		out.Only = *r.Only
	}
	if r.Ignore != nil {
		out.Ignore = *r.Ignore
	}
	if r.Quantizations != nil {
		out.Quantizations = *r.Quantizations
	}
	if r.Sort != nil {
		out.Sort = *r.Sort
	}
	if r.PreferredMaxLatencyP90 != nil {
		out.PreferredMaxLatencyP90 = *r.PreferredMaxLatencyP90
	}
	if r.ReasoningEffort != nil {
		out.ReasoningEffort = *r.ReasoningEffort
	}
	return out
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
	available, err := h.store.ListAvailableModels(r.Context(), provider, tier, top)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractAvailableModels(available))
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
