// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// SetAiProviderSettings replaces one provider's entry. Human-only for the same
// reason ReplaceAiRouting is: a host decides where the installation's text goes.
func (h aiRoutingHandlers) SetAiProviderSettings(w http.ResponseWriter, r *http.Request, provider string) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "SetAiProviderSettings")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.AiProviderSettings
	if !httperr.DecodeClosed(w, r, &req) {
		return
	}
	cfg, err := h.store.SetProviderSettings(r.Context(), provider, providerSettingsFromWire(req))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeAiRouting(w, r, cfg)
}

// providersToWire and providersFromWire carry the providers map. Nil on the
// way in is "keep the stored entries", which an old client relies on, so an
// absent map never becomes an empty one or the reverse.
func providersToWire(in map[string]ai.ProviderSettings) *map[string]crmcontracts.AiProviderSettings {
	if in == nil {
		return nil
	}
	out := make(map[string]crmcontracts.AiProviderSettings, len(in))
	for name, p := range in {
		out[name] = crmcontracts.AiProviderSettings{BaseUrl: optionalString(p.BaseURL), Upstream: upstreamToWire(p.Upstream), Location: optionalString(p.Location)}
	}
	return &out
}

func providersFromWire(in *map[string]crmcontracts.AiProviderSettings) map[string]ai.ProviderSettings {
	if in == nil {
		return nil
	}
	out := make(map[string]ai.ProviderSettings, len(*in))
	for name, p := range *in {
		out[name] = providerSettingsFromWire(p)
	}
	return out
}

func providerSettingsFromWire(p crmcontracts.AiProviderSettings) ai.ProviderSettings {
	out := ai.ProviderSettings{Upstream: upstreamFromWire(p.Upstream)}
	if p.BaseUrl != nil {
		out.BaseURL = *p.BaseUrl
	}
	if p.Location != nil {
		out.Location = *p.Location
	}
	return out
}

// upstreamToWire and upstreamFromWire keep absent and empty apart, as
// routingToWire does: an empty upstream says "no pins", absent says nothing.
// The connection holds only its own keys, so the flat contract shape says all
// of it.
func upstreamToWire(r *ai.OpenRouterRouting) *crmcontracts.AiOpenRouterUpstream {
	if r == nil {
		return nil
	}
	p := r.Provider
	return &crmcontracts.AiOpenRouterUpstream{
		Only: optionalStrings(p.Only), Ignore: optionalStrings(p.Ignore), AllowFallbacks: p.AllowFallbacks,
		Zdr: p.ZDR, EnforceDistillableText: p.EnforceDistillableText,
		DataCollection: optionalEnum[crmcontracts.AiOpenRouterUpstreamDataCollection](p.DataCollection),
	}
}

func upstreamFromWire(r *crmcontracts.AiOpenRouterUpstream) *ai.OpenRouterRouting {
	if r == nil {
		return nil
	}
	out := &ai.OpenRouterRouting{Provider: ai.OpenRouterProvider{
		AllowFallbacks: r.AllowFallbacks, ZDR: r.Zdr, EnforceDistillableText: r.EnforceDistillableText,
	}}
	if r.Only != nil {
		out.Provider.Only = *r.Only
	}
	if r.Ignore != nil {
		out.Provider.Ignore = *r.Ignore
	}
	if r.DataCollection != nil {
		out.Provider.DataCollection = string(*r.DataCollection)
	}
	return out
}
