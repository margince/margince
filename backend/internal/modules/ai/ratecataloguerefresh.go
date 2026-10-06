// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The price sync's store half: apply a plan over every price source in one
// transaction, and say per provider what it did.

import (
	"context"
	"errors"
	"maps"
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
	// RefreshNotListed means a bound model is absent from the catalogue, so
	// there is nothing to read for it and nothing to set by hand either.
	RefreshNotListed RefreshOutcome = "not_listed"
	// RefreshUnreachable means the catalogue could not be read.
	RefreshUnreachable RefreshOutcome = "unreachable"
	// RefreshNotBound means nothing the provider serves is bound or on the sheet.
	RefreshNotBound RefreshOutcome = "not_bound"
	// RefreshNotConfigured means the provider holds no usable key, so it was not touched.
	RefreshNotConfigured RefreshOutcome = "not_configured"
)

// ProviderRefresh is one provider's line of a sync report. Tagged because the
// last run is stored as a setting and read back by a later build.
type ProviderRefresh struct {
	Provider  string         `json:"provider"`
	Outcome   RefreshOutcome `json:"outcome"`
	Updated   int            `json:"updated"`
	Unchanged int            `json:"unchanged"`
	// Added is the models priced for the first time; Kept the hand-set ones left alone.
	Added int `json:"added"`
	Kept  int `json:"kept"`
	// Unlisted is the models on the sheet the catalogue does not name, in id order.
	Unlisted []string `json:"unlisted"`
	// Models is the ids written this run, in id order.
	Models []string `json:"models"`
}

// RateRefreshReport lists every provider this build knows, chat adapters
// first and decision-only adapters after, in registry order.
type RateRefreshReport struct {
	Providers []ProviderRefresh `json:"providers"`
}

// catalogueTarget is one model the refresh may re-price. lane is empty for a
// model known only from the sheet, so the write inherits the lane it is filed under.
type catalogueTarget struct {
	provider, modelID string
	lane              Lane
}

// catalogueTargets is the scope of a refresh: the OpenRouter-hosted models the
// routing document binds, plus the openai_compatible models already on the
// sheet. The sheet rows are kept because a model unbound last week still shows
// a price somewhere, and an unbound price left to rot misleads the usage page.
//
// They are kept only while something is bound at OpenRouter. Without that, an
// openai_compatible row belongs to a host this refresh knows nothing about (a
// self-hosted server priced 0 by hand), and an id it happens to share with an
// OpenRouter model is a coincidence, not a price. Tiers are visited in name
// order and the result is sorted by provider then model, so a run is reproducible.
// Each lane is read at its provider's host, resolved through the lift so a
// document in either shape is judged the same way.
func catalogueTargets(cfg RoutingConfig, sheet []ModelRateRow) []catalogueTarget {
	cfg = cfg.canonical().resolveProviders()
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
	atOpenRouter := false
	for _, name := range slices.Sorted(maps.Keys(cfg.Tiers)) {
		if tier := cfg.Tiers[name]; openRouterChat(tier) {
			atOpenRouter = true
			add(catalogueTarget{tier.Provider, tier.Model, LaneChat})
		}
	}
	if openRouterChat(cfg.Embeddings.ProviderConfig) {
		atOpenRouter = true
		add(catalogueTarget{cfg.Embeddings.Provider, cfg.Embeddings.Model, LaneEmbeddings})
	}
	if d := cfg.Decisions; d != nil && decisionHostFor(d.Provider, d.BaseURL) == decisionHostOpenRouter {
		add(catalogueTarget{d.Provider, d.Model, LaneDecisions})
	}
	for _, row := range sheet {
		if atOpenRouter && row.Provider == providerOpenAICompatible {
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

// SyncPrices writes today's catalogue price for every in-scope model whose price
// in force differs, and hands the report to record in the same transaction, so a
// failed run leaves neither prices nor a record of having run.
func (s *RateStore) SyncPrices(ctx context.Context, src PriceSources, record func(context.Context, pgx.Tx, RateRefreshReport) error) (RateRefreshReport, error) {
	if err := auth.RequireAny(ctx, "ai_model_rate", principal.ActionCreate, principal.ActionUpdate); err != nil {
		return RateRefreshReport{}, err
	}
	sheet, err := s.ListEffectiveModelRates(ctx)
	if err != nil {
		return RateRefreshReport{}, err
	}
	plan := planPriceSync(src, sheet)
	var report RateRefreshReport
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, w := range plan.writes {
			_, err := s.SetModelRateInTx(ctx, tx, w)
			switch {
			case errors.Is(err, errHandSetSinceRead):
				plan.yielded(w)
			case err != nil:
				return err
			}
		}
		// Built after the writes, so the recorded run says what was written.
		report = reportProviders(plan.lines, src.Broker.Unavailable != "")
		return record(ctx, tx, report)
	}); err != nil {
		return RateRefreshReport{}, err
	}
	return report, nil
}

// reportProviders gives every known provider one line, in registry order.
func reportProviders(lines map[string]*ProviderRefresh, brokerDown bool) RateRefreshReport {
	names := KnownProviders()
	for _, d := range DecisionProviders() {
		if !slices.Contains(names, d) {
			names = append(names, d)
		}
	}
	out := make([]ProviderRefresh, 0, len(names))
	for _, name := range names {
		out = append(out, reportLine(name, lines[name], brokerDown))
	}
	return RateRefreshReport{Providers: out}
}

func reportLine(name string, planned *ProviderRefresh, brokerDown bool) ProviderRefresh {
	_, vendor := modelsDevKeyFor(name)
	line := ProviderRefresh{Provider: name}
	if planned != nil {
		line = *planned
	}
	switch {
	case !vendor && !brokerPriced(name):
		line = ProviderRefresh{Provider: name, Outcome: RefreshNotAvailable}
	case planned == nil:
		line.Outcome = RefreshNotBound
	case line.Outcome != "":
		// Decided while planning: out of scope, or its catalogue was unreadable.
	case brokerPriced(name) && brokerDown:
		line = ProviderRefresh{Provider: name, Outcome: RefreshUnreachable}
	case line.Updated+line.Added > 0:
		line.Outcome = RefreshUpdated
	case line.Unchanged+line.Kept > 0:
		line.Outcome = RefreshUnchanged
	case len(line.Unlisted) > 0:
		line.Outcome = RefreshNotListed
	case vendor:
		// Keyed, but nothing on its sheet and nothing its key lists to price.
		line.Outcome = RefreshNotBound
	default:
		line.Outcome = RefreshNotAvailable
	}
	// The wire promises lists, never null.
	if line.Models == nil {
		line.Models = []string{}
	}
	if line.Unlisted == nil {
		line.Unlisted = []string{}
	}
	return line
}
