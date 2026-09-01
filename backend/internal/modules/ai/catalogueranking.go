// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The OpenRouter response shape, and the ranking this module derives from
// it: keep only models a public benchmark actually scored, sort by that
// score, collapse a model's billing-lane aliases (":batch", ":free"...)
// onto the one bindable id, and price the survivors in the contract's own
// USD-per-MTok decimal strings.

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// openRouterCatalogue is the vendor's own envelope: one entry per model.
type openRouterCatalogue struct {
	Data []openRouterModel `json:"data"`
}

type openRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength *int   `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Benchmarks struct {
		ArtificialAnalysis struct {
			IntelligenceIndex *float64 `json:"intelligence_index"`
		} `json:"artificial_analysis"`
	} `json:"benchmarks"`
}

// parseOpenRouterCatalogue turns a vendor response body into the wire
// response: ranked, deduped, priced, capped at catalogueTop. fetchedAt is
// the clock sample taken right before the request, carried through so a
// later cache hit reports when the vendor was actually read.
func parseOpenRouterCatalogue(body []byte, fetchedAt time.Time) (crmcontracts.AiModelCatalogueResponse, error) {
	var vendor openRouterCatalogue
	if err := json.Unmarshal(body, &vendor); err != nil {
		return crmcontracts.AiModelCatalogueResponse{}, fmt.Errorf("ai model catalogue: unparseable openrouter response: %w", err)
	}

	entries := make([]crmcontracts.AiModelCatalogueEntry, 0, catalogueTop)
	for _, m := range rankOpenRouterModels(vendor.Data) {
		entry, priced := toCatalogueEntry(m)
		if !priced {
			// A model whose price will not parse is dropped rather than shown
			// broken or free: better a shorter list than a wrong number.
			continue
		}
		entries = append(entries, entry)
		if len(entries) == catalogueTop {
			break
		}
	}

	fetched := fetchedAt
	return crmcontracts.AiModelCatalogueResponse{
		Data: entries, RankedBy: catalogueRankedBy, Unavailable: false, FetchedAt: &fetched,
	}, nil
}

// rankOpenRouterModels keeps only benchmarked models, sorts them by score
// descending, and collapses a model's billing-lane variants onto its one
// bindable id (the part before the first ":"), keeping the highest-scoring
// among them. A tie between a model and its own variant is broken toward
// the bare id, which is the one a screen can actually bind.
func rankOpenRouterModels(models []openRouterModel) []openRouterModel {
	scored := make([]openRouterModel, 0, len(models))
	for _, m := range models {
		if m.Benchmarks.ArtificialAnalysis.IntelligenceIndex != nil {
			scored = append(scored, m)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		si := *scored[i].Benchmarks.ArtificialAnalysis.IntelligenceIndex
		sj := *scored[j].Benchmarks.ArtificialAnalysis.IntelligenceIndex
		if si != sj {
			return si > sj
		}
		return isBareModelID(scored[i].ID) && !isBareModelID(scored[j].ID)
	})

	seen := map[string]bool{}
	ranked := make([]openRouterModel, 0, len(scored))
	for _, m := range scored {
		base := baseModelID(m.ID)
		if seen[base] {
			// Sorted descending, ties broken toward the bare id above, so the
			// first sighting of a base is already the one to keep.
			continue
		}
		seen[base] = true
		ranked = append(ranked, m)
	}
	return ranked
}

func baseModelID(id string) string {
	base, _, _ := strings.Cut(id, ":")
	return base
}

func isBareModelID(id string) bool {
	return baseModelID(id) == id
}

// toCatalogueEntry converts one vendor model into the wire entry, reporting
// false when either price will not parse (the entry must then be dropped,
// never rendered with a broken or zero price).
func toCatalogueEntry(m openRouterModel) (crmcontracts.AiModelCatalogueEntry, bool) {
	inputPerMtok, ok := tokenPriceToUsdPerMTok(m.Pricing.Prompt)
	if !ok {
		return crmcontracts.AiModelCatalogueEntry{}, false
	}
	outputPerMtok, ok := tokenPriceToUsdPerMTok(m.Pricing.Completion)
	if !ok {
		return crmcontracts.AiModelCatalogueEntry{}, false
	}
	score := strconv.FormatFloat(*m.Benchmarks.ArtificialAnalysis.IntelligenceIndex, 'f', 1, 64)
	return crmcontracts.AiModelCatalogueEntry{
		ContextLength: m.ContextLength,
		InputPerMtok:  inputPerMtok,
		ModelId:       m.ID,
		Name:          m.Name,
		OutputPerMtok: outputPerMtok,
		RankScore:     &score,
	}, true
}

// tokenPriceToUsdPerMTok converts a vendor's USD-per-single-token decimal
// string into the contract's USD-per-million-tokens display string. It
// reuses the round-half-up-and-guard step UsdPerMTokToMicroUSD uses (via
// scaleRatToMicroUSD) and the same MicroUSDToUsdPerMTok formatter; only the
// scale differs, because the vendor's own price is per token, not per MTok.
// A per-token price commonly needs many fractional digits ("0.00000015" for
// $0.15/MTok), so the fractional cap is far wider than money.go's rate-sheet
// one; the integer cap stays tight since a legitimate per-token price never
// needs many whole-dollar digits.
func tokenPriceToUsdPerMTok(usdPerToken string) (string, bool) {
	s := strings.TrimSpace(usdPerToken)
	if s != usdPerToken || !values.PlainDecimal(s, 6, 18) {
		return "", false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", false
	}
	r.Mul(r, new(big.Rat).SetInt64(usdPerTokenToMicroUSDPerMTok))
	micro, ok := scaleRatToMicroUSD(r)
	if !ok {
		return "", false
	}
	return MicroUSDToUsdPerMTok(micro), true
}
