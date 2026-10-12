// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"cmp"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PriceSources is everything one sync run reads before it writes anything, so
// the write transaction holds no network call.
type PriceSources struct {
	Routing RoutingConfig
	// Broker is OpenRouter's full list, which prices the broker-carried models.
	Broker    AvailableModels
	ModelsDev ModelsDevPrices
	// Usable is ProviderKeyStatus.Usable by provider; a vendor absent is out of scope.
	Usable map[string]bool
	// Listed is what each usable vendor's key lists; absent when the listing failed.
	Listed map[string][]model.Info
}

type pricePlan struct {
	writes []SetModelRateInput
	lines  map[string]*ProviderRefresh
	// added is the writes that price a model for the first time, by provider/model.
	added map[[2]string]bool
}

func (p *pricePlan) line(provider string) *ProviderRefresh {
	if p.lines[provider] == nil {
		p.lines[provider] = &ProviderRefresh{Provider: provider}
	}
	return p.lines[provider]
}

// apply is the one decision every target takes, whichever catalogue priced it:
// a hand-set price is kept, a matching one counted, anything else written.
func (p *pricePlan) apply(t catalogueTarget, cur *ModelRateRow, price func(*ModelRateRow) (SetModelRateInput, bool)) {
	line := p.line(t.provider)
	if cur != nil && cur.Source == RateSourceManual {
		line.Kept++
		return
	}
	next, ok := price(cur)
	switch {
	case !ok:
		return
	case cur != nil && sameMicroUSD(rateInput(*cur), next):
		line.Unchanged++
		return
	case cur != nil:
		line.Updated++
	default:
		line.Added++
		p.added[[2]string{t.provider, t.modelID}] = true
	}
	next.Provider, next.ModelID, next.Lane, next.Source = t.provider, t.modelID, t.lane, RateSourceCatalogue
	p.writes = append(p.writes, next)
	line.Models = append(line.Models, t.modelID)
}

// yielded re-reports a planned write that found a hand-set price under its lock:
// the model was kept, not priced.
func (p *pricePlan) yielded(w SetModelRateInput) {
	line := p.line(w.Provider)
	if p.added[[2]string{w.Provider, w.ModelID}] {
		line.Added--
	} else {
		line.Updated--
	}
	line.Kept++
	line.Models = slices.DeleteFunc(line.Models, func(id string) bool { return id == w.ModelID })
}

// planPriceSync decides every write before any is made, so the transaction
// that follows only applies it.
func planPriceSync(src PriceSources, sheet []ModelRateRow) pricePlan {
	plan := pricePlan{lines: map[string]*ProviderRefresh{}, added: map[[2]string]bool{}}
	inForce := make(map[[2]string]ModelRateRow, len(sheet))
	for _, row := range sheet {
		inForce[[2]string{row.Provider, row.ModelID}] = row
	}
	plan.broker(src, sheet, inForce)
	for _, provider := range modelsDevProviders() {
		plan.vendor(src, provider, sheet, inForce)
	}
	return plan
}

func inForceRow(inForce map[[2]string]ModelRateRow, t catalogueTarget) *ModelRateRow {
	if row, ok := inForce[[2]string{t.provider, t.modelID}]; ok {
		return &row
	}
	return nil
}

func (p *pricePlan) broker(src PriceSources, sheet []ModelRateRow, inForce map[[2]string]ModelRateRow) {
	listed := make(map[string]AvailableModel, len(src.Broker.Models))
	for _, m := range src.Broker.Models {
		listed[m.ID] = m
	}
	for _, t := range catalogueTargets(src.Routing, sheet) {
		entry, ok := listed[t.modelID]
		if !ok {
			if line := p.line(t.provider); t.lane != "" && src.Broker.Unavailable == "" {
				line.Unlisted = append(line.Unlisted, t.modelID)
			}
			continue
		}
		p.apply(t, inForceRow(inForce, t), func(*ModelRateRow) (SetModelRateInput, bool) {
			return cataloguePrice(entry, t)
		})
	}
}

// vendor prices one models.dev provider: its sheet rows, plus each model its key
// lists that the catalogue prices. Out of scope or unreadable, it writes nothing.
func (p *pricePlan) vendor(src PriceSources, provider string, sheet []ModelRateRow, inForce map[[2]string]ModelRateRow) {
	line := p.line(provider)
	key, _ := modelsDevKeyFor(provider)
	switch {
	case !src.Usable[provider]:
		line.Outcome = RefreshNotConfigured
		return
	case src.ModelsDev.Unreachable:
		line.Outcome = RefreshUnreachable
		return
	}
	for _, t := range vendorTargets(src, provider, key, sheet) {
		entry, ok := src.ModelsDev.entry(key, t.modelID)
		if !ok {
			line.Unlisted = append(line.Unlisted, t.modelID)
			continue
		}
		p.apply(t, inForceRow(inForce, t), entry.price)
	}
}

// vendorTargets is one vendor's scope in id order: its sheet rows filed where
// they are, and the listed models the catalogue prices, in the lane both agree on.
func vendorTargets(src PriceSources, provider, key string, sheet []ModelRateRow) []catalogueTarget {
	byID := map[string]catalogueTarget{}
	for _, row := range sheet {
		if row.Provider == provider {
			byID[row.ModelID] = catalogueTarget{provider: provider, modelID: row.ModelID}
		}
	}
	for _, info := range src.Listed[provider] {
		entry, priced := src.ModelsDev.entry(key, info.ID)
		if _, onSheet := byID[info.ID]; onSheet || !priced {
			continue
		}
		if lane, ok := syncLane(info, entry); ok {
			byID[info.ID] = catalogueTarget{provider: provider, modelID: info.ID, lane: lane}
		}
	}
	out := slices.Collect(maps.Values(byID))
	slices.SortFunc(out, func(a, b catalogueTarget) int { return cmp.Compare(a.modelID, b.modelID) })
	return out
}

// syncLane is the lane a listed model is added under: the catalogue's, which a
// lane the vendor states must agree with.
func syncLane(info model.Info, entry modelsDevModel) (Lane, bool) {
	lane, ok := entry.lane()
	if !ok || (info.Lane != "" && Lane(info.Lane) != lane) {
		return "", false
	}
	return lane, true
}
