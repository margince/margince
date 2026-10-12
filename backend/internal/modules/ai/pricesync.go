// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// PriceSyncDeps is what the engine reads and writes through; compose builds it
// once for the api and once for the worker.
type PriceSyncDeps struct {
	Rates     *RateStore
	Routing   *RoutingStore
	Keys      *ProviderKeyStore
	Broker    *ModelCatalogue
	ModelsDev *ModelsDevCatalogue
	Settings  *settings.Store
}

// PriceSync is the one engine behind "Refresh now" and the daily sweep.
type PriceSync struct {
	d   PriceSyncDeps
	now func() time.Time
}

// NewPriceSync builds the engine over its stores and the two catalogues.
func NewPriceSync(d PriceSyncDeps) *PriceSync { return &PriceSync{d: d, now: time.Now} }

// PriceSyncState is what the Model prices card shows.
type PriceSyncState struct {
	AutoSync bool
	LastRun  *LastPriceSync
}

// Run syncs this installation's prices and records the run as trigger.
func (p *PriceSync) Run(ctx context.Context, trigger PriceSyncTrigger) (RateRefreshReport, error) {
	src, err := p.sources(ctx)
	if err != nil {
		return RateRefreshReport{}, err
	}
	return p.d.Rates.SyncPrices(ctx, src, func(ctx context.Context, tx pgx.Tx, report RateRefreshReport) error {
		run := &LastPriceSync{RanAt: p.now().UTC(), Trigger: trigger, Report: report}
		return settings.SetTx(ctx, p.d.Settings, tx, PriceSyncLastRun, run)
	})
}

// RunScheduled is the daily pass: a Run, unless auto-sync is off.
func (p *PriceSync) RunScheduled(ctx context.Context) error {
	cfg, err := settings.Get(ctx, p.d.Settings, PriceSyncSettings)
	if err != nil || !cfg.AutoSync {
		return err
	}
	_, err = p.Run(ctx, PriceSyncScheduled)
	return err
}

// State is the switch and the last run, as the Model prices card shows them.
func (p *PriceSync) State(ctx context.Context) (PriceSyncState, error) {
	cfg, err := settings.Get(ctx, p.d.Settings, PriceSyncSettings)
	if err != nil {
		return PriceSyncState{}, err
	}
	last, err := settings.Get(ctx, p.d.Settings, PriceSyncLastRun)
	if err != nil {
		return PriceSyncState{}, err
	}
	return PriceSyncState{AutoSync: cfg.AutoSync, LastRun: last}, nil
}

// SetAutoSync turns the daily sweep on or off; Refresh now runs either way.
func (p *PriceSync) SetAutoSync(ctx context.Context, on bool) (PriceSyncState, error) {
	// The answer is the state, which takes a read; refuse before writing, not after.
	if err := auth.Require(ctx, "ai_model_rate", principal.ActionRead); err != nil {
		return PriceSyncState{}, err
	}
	if err := settings.Set(ctx, p.d.Settings, PriceSyncSettings, PriceSyncConfig{AutoSync: on}); err != nil {
		return PriceSyncState{}, err
	}
	return p.State(ctx)
}

// sources reads every list before anything is written. A vendor whose own
// listing fails still has its sheet re-priced; it simply adds nothing.
func (p *PriceSync) sources(ctx context.Context) (PriceSources, error) {
	cfg, err := p.d.Routing.Get(ctx)
	if err != nil {
		return PriceSources{}, err
	}
	statuses, err := p.d.Keys.List(ctx)
	if err != nil {
		return PriceSources{}, err
	}
	src := PriceSources{Routing: cfg, Usable: map[string]bool{}, Listed: map[string][]model.Info{}}
	for _, s := range statuses {
		src.Usable[s.Provider] = s.Usable()
	}
	readBroker, readModelsDev := cataloguesToRead(cfg, src.Usable)
	if readBroker {
		src.Broker = p.d.Broker.List(ctx, 0)
	}
	if readModelsDev {
		src.ModelsDev = p.d.ModelsDev.Prices(ctx)
	}
	for _, provider := range modelsDevProviders() {
		if !src.Usable[provider] {
			continue
		}
		listed, err := p.d.Routing.ListAvailableModels(ctx, AvailableModelsQuery{Provider: provider})
		if err != nil {
			return PriceSources{}, err
		}
		if listed.Unavailable == AvailabilityOK {
			src.Listed[provider] = modelInfos(listed.Models)
		}
	}
	return src, nil
}

// cataloguesToRead is which public lists this run needs: OpenRouter's only while
// something is bound there, models.dev only while a vendor it prices is keyed.
func cataloguesToRead(cfg RoutingConfig, usable map[string]bool) (broker, modelsDev bool) {
	broker = len(catalogueTargets(cfg, nil)) > 0
	for _, provider := range modelsDevProviders() {
		modelsDev = modelsDev || usable[provider]
	}
	return broker, modelsDev
}

func modelInfos(models []AvailableModel) []model.Info {
	out := make([]model.Info, len(models))
	for i, m := range models {
		out[i] = m.Info
	}
	return out
}
