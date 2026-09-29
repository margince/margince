// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The catalogue refresh: re-price, from OpenRouter's own published list, the
// models this installation actually calls. Only a broker publishes prices in a
// model-list API, so every other provider reports that its prices are set by
// hand instead of being guessed at.

import (
	"context"
	"slices"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RefreshOutcome is what a refresh did for one provider.
type RefreshOutcome string

const (
	// RefreshUpdated means at least one price was written.
	RefreshUpdated RefreshOutcome = "updated"
	// RefreshUnchanged means every priced model was already at the catalogue price.
	RefreshUnchanged RefreshOutcome = "unchanged"
	// RefreshNotAvailable means the provider publishes no price to read.
	RefreshNotAvailable RefreshOutcome = "not_available"
	// RefreshUnreachable means the catalogue could not be read.
	RefreshUnreachable RefreshOutcome = "unreachable"
	// RefreshNotBound means nothing the provider serves is bound or on the sheet.
	RefreshNotBound RefreshOutcome = "not_bound"
)

// ProviderRefresh is one provider's line of a refresh report.
type ProviderRefresh struct {
	Provider  string
	Outcome   RefreshOutcome
	Updated   int
	Unchanged int
	// Models is the ids written this run, in id order.
	Models []string
}

// RateRefreshReport lists every provider this build knows, chat adapters
// first and decision-only adapters after, in registry order.
type RateRefreshReport struct {
	Providers []ProviderRefresh
}

// catalogueTarget is one model the refresh may re-price. lane is empty for a
// model known only from the sheet, so the write inherits the lane it is filed under.
type catalogueTarget struct {
	provider, modelID string
	lane              Lane
}

// brokerProviders are the providers an OpenRouter binding can carry. A model
// of any other provider is not priced by OpenRouter's list, whatever its id.
var brokerProviders = []string{providerOpenAICompatible, providerJevCompatible}

// catalogueTargets is the scope of a refresh: the OpenRouter-hosted models the
// routing document binds, plus the openai_compatible models already on the
// sheet. The sheet rows are kept because a model unbound last week still shows
// a price somewhere, and an unbound price left to rot misleads the usage page.
// Sorted by provider then model so a run is reproducible.
func catalogueTargets(cfg RoutingConfig, sheet []ModelRateRow) []catalogueTarget {
	byKey := map[[2]string]catalogueTarget{}
	add := func(t catalogueTarget) {
		if t.modelID == "" {
			return
		}
		key := [2]string{t.provider, t.modelID}
		if prior, ok := byKey[key]; ok && prior.lane != "" {
			return
		}
		byKey[key] = t
	}
	for _, tier := range cfg.Tiers {
		if openRouterChat(tier) {
			add(catalogueTarget{tier.Provider, tier.Model, LaneChat})
		}
	}
	if openRouterChat(cfg.Embeddings.ProviderConfig) {
		add(catalogueTarget{cfg.Embeddings.Provider, cfg.Embeddings.Model, LaneEmbeddings})
	}
	if d := cfg.Decisions; d != nil && decisionHostFor(d.Provider, d.BaseURL) == decisionHostOpenRouter {
		add(catalogueTarget{d.Provider, d.Model, LaneDecisions})
	}
	for _, row := range sheet {
		if row.Provider == providerOpenAICompatible {
			add(catalogueTarget{provider: row.Provider, modelID: row.ModelID})
		}
	}
	out := make([]catalogueTarget, 0, len(byKey))
	for _, t := range byKey {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].provider != out[j].provider {
			return out[i].provider < out[j].provider
		}
		return out[i].modelID < out[j].modelID
	})
	return out
}

func openRouterChat(b ProviderConfig) bool {
	return b.Provider == providerOpenAICompatible && IsOpenRouterHost(b.BaseURL)
}

// cataloguePrice is the sheet price the vendor's entry states. Cache buckets
// the vendor does not publish are zero, the sheet's spelling of "not offered".
// False when the vendor states no usable input or output price: a model the
// list names with a variable price (OpenRouter publishes -1) must not be
// written as free.
func cataloguePrice(m AvailableModel, t catalogueTarget) (SetModelRateInput, bool) {
	if m.InputPerMtok == nil || m.OutputPerMtok == nil {
		return SetModelRateInput{}, false
	}
	orZero := func(v *string) string {
		if v == nil {
			return "0"
		}
		return *v
	}
	in := SetModelRateInput{
		Provider: t.provider, ModelID: t.modelID, Lane: t.lane,
		InputUsd: *m.InputPerMtok, OutputUsd: *m.OutputPerMtok,
		CacheReadUsd: orZero(m.CacheReadPerMtok), CacheWriteUsd: orZero(m.CacheWritePerMtok),
	}
	if _, _, _, _, err := modelRateMicroUSD(in); err != nil {
		return SetModelRateInput{}, false
	}
	return in, true
}

// sameMicroUSD compares two prices at the precision the sheet stores, so a
// catalogue figure with more digits than a micro-dollar does not read as a
// change on every run.
func sameMicroUSD(a, b SetModelRateInput) bool {
	ai, ao, ar, aw, errA := modelRateMicroUSD(a)
	bi, bo, br, bw, errB := modelRateMicroUSD(b)
	return errA == nil && errB == nil && ai == bi && ao == bo && ar == br && aw == bw
}

func rateInput(row ModelRateRow) SetModelRateInput {
	return SetModelRateInput{
		InputUsd: row.InputUsd, OutputUsd: row.OutputUsd,
		CacheReadUsd: row.CacheReadUsd, CacheWriteUsd: row.CacheWriteUsd,
	}
}

// RefreshFromCatalogue writes today's catalogue price for every in-scope model
// whose price in force differs, in one transaction so a failed write leaves the
// sheet as it was. A model at its catalogue price writes nothing and leaves no
// audit row, so pressing the button twice is the same as pressing it once.
func (s *RateStore) RefreshFromCatalogue(ctx context.Context, cfg RoutingConfig, catalogue AvailableModels) (RateRefreshReport, error) {
	if err := auth.RequireAny(ctx, "ai_model_rate", principal.ActionCreate, principal.ActionUpdate); err != nil {
		return RateRefreshReport{}, err
	}
	sheet, err := s.ListEffectiveModelRates(ctx)
	if err != nil {
		return RateRefreshReport{}, err
	}
	inForce := make(map[[2]string]ModelRateRow, len(sheet))
	for _, row := range sheet {
		inForce[[2]string{row.Provider, row.ModelID}] = row
	}
	listed := make(map[string]AvailableModel, len(catalogue.Models))
	for _, m := range catalogue.Models {
		listed[m.ID] = m
	}

	lines := map[string]*ProviderRefresh{}
	var writes []SetModelRateInput
	for _, t := range catalogueTargets(cfg, sheet) {
		line := lines[t.provider]
		if line == nil {
			line = &ProviderRefresh{Provider: t.provider}
			lines[t.provider] = line
		}
		entry, ok := listed[t.modelID]
		if !ok {
			continue
		}
		next, ok := cataloguePrice(entry, t)
		if !ok {
			continue
		}
		if cur, has := inForce[[2]string{t.provider, t.modelID}]; has && sameMicroUSD(rateInput(cur), next) {
			line.Unchanged++
			continue
		}
		writes = append(writes, next)
		line.Updated++
		line.Models = append(line.Models, t.modelID)
	}
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, w := range writes {
			if _, err := s.SetModelRateInTx(ctx, tx, w); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return RateRefreshReport{}, err
	}
	return reportProviders(lines, catalogue.Unavailable != ""), nil
}

// reportProviders gives every known provider one line. A provider a broker can
// carry reports what the run did; any other publishes no price list at all.
func reportProviders(lines map[string]*ProviderRefresh, catalogueDown bool) RateRefreshReport {
	names := KnownProviders()
	for _, d := range DecisionProviders() {
		if !slices.Contains(names, d) {
			names = append(names, d)
		}
	}
	out := make([]ProviderRefresh, 0, len(names))
	for _, name := range names {
		line, ok := lines[name]
		switch {
		case !slices.Contains(brokerProviders, name):
			line = &ProviderRefresh{Provider: name, Outcome: RefreshNotAvailable}
		case !ok:
			line = &ProviderRefresh{Provider: name, Outcome: RefreshNotBound}
		case catalogueDown:
			line.Outcome, line.Updated, line.Unchanged, line.Models = RefreshUnreachable, 0, 0, nil
		case line.Updated > 0:
			line.Outcome = RefreshUpdated
		case line.Unchanged > 0:
			line.Outcome = RefreshUnchanged
		default:
			line.Outcome = RefreshNotAvailable
		}
		if line.Models == nil {
			line.Models = []string{}
		}
		out = append(out, *line)
	}
	return RateRefreshReport{Providers: out}
}
