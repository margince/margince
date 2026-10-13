// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PreviewRouting compares both bindings under the same current allowance band,
// lists every routing key the save would refuse, and shows what each tier
// would send OpenRouter. refused carries the routing values the transport
// could not read; it is reported the same way as a key the store refuses.
//
// A refused routing key is an answer, not an error: the editor shows the
// problems beside the impact, so the impact is judged with each refused lane's
// routing left as stored. Any other refusal is the 422 it always was.
func (s *AdminStore) PreviewRouting(ctx context.Context, next RoutingConfig, refused error) (crmcontracts.AiRoutingPreview, error) {
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionRead); err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	if err := auth.Require(ctx, routingSettingsObject, principal.ActionUpdate); err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	if err := auth.Require(ctx, budgetObject, principal.ActionRead); err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	var faults routingFaults
	if refused != nil && !errors.As(refused, &faults) {
		return crmcontracts.AiRoutingPreview{}, refused
	}
	budget, cfg, err := s.observed(ctx)
	if err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	draft, served, faults, err := previewDraft(cfg, next, faults)
	if err != nil {
		return crmcontracts.AiRoutingPreview{}, settings.InvalidValue{Setting: RoutingKey, Code: settings.CodeInvalidValue, Reason: err.Error()}
	}
	effective, err := effectiveRouting(served)
	if err != nil {
		return crmcontracts.AiRoutingPreview{}, err
	}
	out := crmcontracts.AiRoutingPreview{
		CurrentVersion: cfg.Revision(), Features: compareFeatureRoutes(cfg, draft, budget.Band, budget.Band),
		UnusedTiers: unusedTiers(draft), Effective: effective,
	}
	if len(faults) > 0 {
		out.Errors = faults.toContract()
	}
	return out, nil
}

// previewDraft judges next as the write would store it — settled against the
// stored document, which holds the providers an old client never sends. Each
// lane whose routing is refused is judged with its stored routing instead, so
// the rest of the draft still has an impact to show; the refusals are kept.
func previewDraft(cfg, next RoutingConfig, faults routingFaults) (draft, served RoutingConfig, all routingFaults, err error) {
	// The write stops at the first refusing rule; the editor wants every bad
	// key at once, so each lane's value and placement are judged up front.
	faults = append(faults, next.routingValueFaults(cfg.canonical())...)
	// Each pass can only add refusals on lanes not yet restored, so one pass per
	// lane bounds it; two cover a draft whose stored lanes are themselves valid.
	for range 3 {
		candidate := next.withStoredRoutingAt(cfg.canonical(), faults)
		draft, served, err = candidate.replacing(cfg)
		if err == nil {
			err = validateStoredRouting(draft)
		}
		var more routingFaults
		if err == nil || !errors.As(err, &more) {
			return draft, served, faults, err
		}
		faults = append(faults, more...)
	}
	return cfg, RoutingConfig{}, faults, nil
}

// routingValueFaults judges each tier's routing value on its own and where it
// is placed: a connection key it carries must repeat the connection's.
func (next RoutingConfig) routingValueFaults(stored RoutingConfig) routingFaults {
	providers := next.Providers
	if providers == nil {
		providers = stored.Providers
	}
	var errs []error
	for _, tier := range next.sortedTiers() {
		lane, path := next.Tiers[tier], TierRoutingPath(tier)
		errs = append(errs, lane.Routing.Validate(path))
		if pins := lane.Routing.pins(); pins != nil {
			if held := providers[lane.Provider].Upstream.pins(); held == nil || !samePins(held, pins) {
				errs = append(errs, refuseConnectionKeysOnTier(path, pins))
			}
		}
	}
	return faultsOf(errs...)
}

// withStoredRoutingAt is next with each refused routing value, a connection's
// upstream among them, replaced by the stored one at the same path.
func (next RoutingConfig) withStoredRoutingAt(stored RoutingConfig, faults routingFaults) RoutingConfig {
	if len(faults) == 0 {
		return next
	}
	tiers := make(map[Tier]ProviderConfig, len(next.Tiers))
	for tier, lane := range next.Tiers {
		if faults.under(TierRoutingPath(tier)) {
			lane.Routing = stored.Tiers[tier].Routing.clone()
		}
		tiers[tier] = lane
	}
	if next.Tiers != nil {
		next.Tiers = tiers
	}
	if faults.under(EmbeddingsRoutingPath) {
		next.Embeddings.Routing = stored.Embeddings.Routing.clone()
	}
	providers := make(map[string]ProviderSettings, len(next.Providers))
	for name, settings := range next.Providers {
		if faults.under("providers." + name + ".upstream") {
			settings.Upstream = stored.Providers[name].Upstream.clone()
		}
		providers[name] = settings
	}
	if next.Providers != nil {
		next.Providers = providers
	}
	return next
}

// under reports whether any fault sits at or below path.
func (f routingFaults) under(path string) bool {
	for _, fault := range f {
		if fault.Path == path || strings.HasPrefix(fault.Path, path+".") {
			return true
		}
	}
	return false
}

func (f routingFaults) toContract() *[]crmcontracts.AiFieldError {
	out := make([]crmcontracts.AiFieldError, 0, len(f))
	for _, refusal := range f.FieldFaults() {
		out = append(out, crmcontracts.AiFieldError{Field: refusal.Field, Code: refusal.Code, Message: refusal.Message})
	}
	return &out
}

// effectiveRouting is what each tier of a served binding sends OpenRouter:
// the shipped default, the tier's own keys and the connection's merged, as the
// adapter renders them. Tiers that send no block are left out.
func effectiveRouting(served RoutingConfig) (*crmcontracts.AiRoutingEffective, error) {
	tiers := map[string]crmcontracts.AiOpenRouterRouting{}
	for tier, lane := range served.Tiers {
		routing := lane.Routing
		if !UpstreamPreferencesApply(lane) || routing.IsEmpty() {
			continue
		}
		raw, err := routing.RequestJSON()
		if err != nil {
			return nil, err
		}
		var wire crmcontracts.AiOpenRouterRouting
		if err := json.Unmarshal(raw, &wire); err != nil {
			return nil, err
		}
		tiers[string(tier)] = wire
	}
	return &crmcontracts.AiRoutingEffective{Tiers: tiers}, nil
}
