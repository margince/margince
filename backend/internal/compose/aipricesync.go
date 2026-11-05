// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
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
// daily sweep alike. Each vendor's key is read through its setting, sealed in vault.
func newAIPriceSync(pool *pgxpool.Pool, vault keyvault.Vault, log *slog.Logger, cats aiPriceCatalogues) *ai.PriceSync {
	store, stored := NewSettingsStore(pool), StoredConfig(pool)
	sealed := WithVault(stored, vault, log)
	return ai.NewPriceSync(ai.PriceSyncDeps{
		Rates:     ai.NewRateStore(InstallationDB(pool)),
		Routing:   ai.NewRoutingStore(store, sealed).WithCatalogue(cats.broker),
		Keys:      ai.NewProviderKeyStore(sealed),
		Broker:    cats.broker,
		ModelsDev: cats.modelsDev,
		Settings:  store,
		Config:    stored,
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
		sealed := WithVault(StoredConfig(pool), s.vault, s.log)
		routing := ai.NewRoutingStore(NewSettingsStore(pool), sealed).WithCatalogue(s.priceCatalogues.broker)
		s.aiRoutingHandlers = aiRoutingHandlers{store: routing}
		s.voiceHandlers = s.WithPriceSync(newAIPriceSync(pool, s.vault, s.log, s.priceCatalogues))
	}
}
