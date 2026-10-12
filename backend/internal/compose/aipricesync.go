// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// aiPriceCatalogues are the two public price lists, built once per process so
// the screens and the sweep share one cached read of each.
type aiPriceCatalogues struct {
	broker    *ai.ModelCatalogue
	modelsDev *ai.ModelsDevCatalogue
}

func newAIPriceCatalogues() aiPriceCatalogues {
	return aiPriceCatalogues{broker: ai.NewModelCatalogue(systemClock{}), modelsDev: ai.NewModelsDevCatalogue(systemClock{})}
}

// newAIPriceSync is the one assembly of the price sync, for "Refresh now" and the
// daily sweep alike. env answers for a key the vault has not sealed yet.
func newAIPriceSync(pool *pgxpool.Pool, vault keyvault.Vault, env config.Lookup, log *slog.Logger, cats aiPriceCatalogues) *ai.PriceSync {
	store := NewSettingsStore(pool)
	return ai.NewPriceSync(ai.PriceSyncDeps{
		Rates:     ai.NewRateStore(InstallationDB(pool)),
		Routing:   ai.NewRoutingStore(store, env).WithCatalogue(cats.broker).WithVault(vault),
		Keys:      ai.NewProviderKeyStore(store, vault, log),
		Broker:    cats.broker,
		ModelsDev: cats.modelsDev,
		Settings:  store,
	})
}

// WithPriceCatalogues reads the two public price lists through these fetchers
// instead of the network, for the picker and the price sync alike.
func WithPriceCatalogues(broker, modelsDev ai.CatalogueFetcher) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.priceCatalogues = aiPriceCatalogues{
			broker:    ai.NewModelCatalogueOver(broker, systemClock{}),
			modelsDev: ai.NewModelsDevCatalogueOver(modelsDev, systemClock{}),
		}
		routing := ai.NewRoutingStore(NewSettingsStore(pool), config.FromOS).WithCatalogue(s.priceCatalogues.broker)
		if s.vault != nil {
			routing = routing.WithVault(s.vault)
		}
		s.aiRoutingHandlers = aiRoutingHandlers{store: routing}
		s.voiceHandlers = s.WithPriceSync(newAIPriceSync(pool, s.vault, config.FromOS, s.log, s.priceCatalogues))
	}
}
