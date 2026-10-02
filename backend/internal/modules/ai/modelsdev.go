// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// models.dev's api.json: the public, MIT-licensed price sheet for the vendor
// APIs, already in USD per million tokens. It prices; it never decides scope.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"strings"
)

const (
	modelsDevVendor = "models.dev"
	modelsDevURL    = "https://models.dev/api.json"
	// modelsDevTextOutput is the modality a chat model emits and nothing else.
	modelsDevTextOutput = "text"
	// modelsDevMaxResponseBytes caps the read: the file is 5.3 MB today.
	modelsDevMaxResponseBytes = 16 << 20
)

type modelsDevCost struct {
	Input      *json.Number `json:"input"`
	Output     *json.Number `json:"output"`
	CacheRead  *json.Number `json:"cache_read"`
	CacheWrite *json.Number `json:"cache_write"`
}

// modelsDevModel is one entry; figures stay json.Number so the decimal the
// file wrote is the decimal the sheet parses.
type modelsDevModel struct {
	ID         string `json:"id"`
	Family     string `json:"family"`
	Modalities struct {
		Output []string `json:"output"`
	} `json:"modalities"`
	Cost *modelsDevCost `json:"cost"`
}

// ModelsDevPrices is one read of the catalogue, by catalogue key then model id.
type ModelsDevPrices struct {
	Unreachable bool
	byKey       map[string]map[string]modelsDevModel
}

func (p ModelsDevPrices) entry(catalogueKey, modelID string) (modelsDevModel, bool) {
	m, ok := p.byKey[catalogueKey][modelID]
	return m, ok
}

// ModelsDevCatalogue serves api.json from the 15-minute cache, so a fleet sweep
// reads the file once.
type ModelsDevCatalogue struct {
	cache *catalogueCache[map[string]map[string]modelsDevModel]
}

// NewModelsDevCatalogue wires the production read of models.dev.
func NewModelsDevCatalogue(clock Clock) *ModelsDevCatalogue {
	return NewModelsDevCatalogueOver(vendorCatalogueFetcher{
		vendor: modelsDevVendor, url: modelsDevURL, maxBytes: modelsDevMaxResponseBytes,
	}, clock)
}

// NewModelsDevCatalogueOver is NewModelsDevCatalogue over another fetcher: a test's fake.
func NewModelsDevCatalogueOver(fetcher CatalogueFetcher, clock Clock) *ModelsDevCatalogue {
	return &ModelsDevCatalogue{cache: &catalogueCache[map[string]map[string]modelsDevModel]{
		fetcher: fetcher, clock: clock, parse: parseModelsDev,
	}}
}

// Prices never fails: an unreadable catalogue is Unreachable, logged for an
// operator, and the sync writes nothing from it.
func (c *ModelsDevCatalogue) Prices(ctx context.Context) ModelsDevPrices {
	byKey, err := c.cache.fresh(ctx)
	if err != nil {
		slog.WarnContext(ctx, "models.dev catalogue unusable; pricing nothing from it", "error", err)
		return ModelsDevPrices{Unreachable: true}
	}
	return ModelsDevPrices{byKey: byKey}
}

func parseModelsDev(body []byte) (map[string]map[string]modelsDevModel, error) {
	var file map[string]struct {
		Models map[string]modelsDevModel `json:"models"`
	}
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, fmt.Errorf("ai model catalogue: models.dev answered unreadable JSON: %w", err)
	}
	if len(file) == 0 {
		return nil, errors.New("ai model catalogue: models.dev answered with no providers")
	}
	out := make(map[string]map[string]modelsDevModel, len(file))
	for key, provider := range file {
		out[key] = provider.Models
	}
	return out, nil
}

// A catalogue figure is a short decimal; past these bounds it is a corrupt or
// hostile file, never a price, and no model costs a million dollars per MTok.
const maxCatalogueFigureLen = 32

var maxCatalogueUsdPerMTok = big.NewRat(1_000_000, 1)

// usdPerMTok re-spells a JSON number as the sheet's plain decimal of six places;
// the file may write an exponent the sheet's parser refuses.
func usdPerMTok(n json.Number) (string, bool) {
	if len(n) > maxCatalogueFigureLen {
		return "", false
	}
	r, ok := new(big.Rat).SetString(n.String())
	if !ok || r.Sign() < 0 || r.Cmp(maxCatalogueUsdPerMTok) > 0 {
		return "", false
	}
	// A non-zero figure below the sheet's sixth place would be stored as free.
	spelled := r.FloatString(6)
	if r.Sign() > 0 && spelled == "0.000000" {
		return "", false
	}
	return spelled, true
}

// lane is what the sheet files this model as: an embedder by name, a chat model
// when its only output is text. Image, audio or video output is neither.
func (m modelsDevModel) lane() (Lane, bool) {
	if strings.Contains(m.ID, "embedding") || strings.Contains(m.Family, "embedding") {
		return LaneEmbeddings, true
	}
	if slices.Equal(m.Modalities.Output, []string{modelsDevTextOutput}) {
		return LaneChat, true
	}
	return "", false
}

// price lays the stated figures over current: a figure the file omits keeps the
// price in force, and is 0 only for a model with no row. No input is no price.
func (m modelsDevModel) price(current *ModelRateRow) (SetModelRateInput, bool) {
	if m.Cost == nil || m.Cost.Input == nil {
		return SetModelRateInput{}, false
	}
	next := SetModelRateInput{InputUsd: "0", OutputUsd: "0", CacheReadUsd: "0", CacheWriteUsd: "0"}
	if current != nil {
		next = rateInput(*current)
	}
	for _, f := range []struct {
		stated *json.Number
		into   *string
	}{
		{m.Cost.Input, &next.InputUsd},
		{m.Cost.Output, &next.OutputUsd},
		{m.Cost.CacheRead, &next.CacheReadUsd},
		{m.Cost.CacheWrite, &next.CacheWriteUsd},
	} {
		if f.stated == nil {
			continue
		}
		v, ok := usdPerMTok(*f.stated)
		if !ok {
			return SetModelRateInput{}, false
		}
		*f.into = v
	}
	if _, _, _, _, err := modelRateMicroUSD(next); err != nil {
		return SetModelRateInput{}, false
	}
	return next, true
}
