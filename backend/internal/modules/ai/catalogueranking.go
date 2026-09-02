// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The OpenRouter response shape, and the two views this module derives from
// it: the FULL vendor list, in the vendor's own order and priced where the
// vendor prices it, and the RANKED view — keep only models a public benchmark
// actually scored, sort by that score, collapse a model's billing-lane
// aliases (":batch", ":free"...) onto the one bindable id, and cap at the
// caller's `top`.

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/model"
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

// parseOpenRouterCatalogue turns a vendor response body into the vendor's own
// model list, in the vendor's own order. No ranking, dedup or pricing is done
// here — that is for the caller's chosen view (rankedAvailableModels or
// fullAvailableModels) to apply, since which one applies depends on the
// request's `top` rather than on the read itself.
func parseOpenRouterCatalogue(body []byte) ([]openRouterModel, error) {
	var vendor openRouterCatalogue
	if err := json.Unmarshal(body, &vendor); err != nil {
		return nil, fmt.Errorf("ai model catalogue: unparseable openrouter response: %w", err)
	}
	return vendor.Data, nil
}

// rankedAvailableModels keeps only benchmarked models, ranks them by score
// descending, collapses each model's billing-lane aliases onto its one
// bindable id, prices the survivors and caps the result at `top`. A model
// whose price will not parse is dropped rather than shown broken or free:
// better a shorter list than a wrong number.
func rankedAvailableModels(models []openRouterModel, top int) ([]AvailableModel, string) {
	entries := make([]AvailableModel, 0, top)
	for _, m := range rankOpenRouterModels(models) {
		entry, priced := toRankedModel(m)
		if !priced {
			continue
		}
		entries = append(entries, entry)
		if len(entries) == top {
			break
		}
	}
	return entries, catalogueRankedBy
}

// fullAvailableModels answers every model the vendor listed, in the vendor's
// own order and with no ranking applied. A model whose price will not parse
// is still shown, with the price fields simply absent: "the vendor's whole
// list" means every id it named, not only the ones this adapter could price.
func fullAvailableModels(models []openRouterModel) []AvailableModel {
	entries := make([]AvailableModel, 0, len(models))
	for _, m := range models {
		entries = append(entries, toFullModel(m))
	}
	return entries
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

// toRankedModel converts one vendor model into the wire entry for a ranked
// view, reporting false when either price will not parse — the entry must
// then be dropped, never rendered with a broken or zero price.
func toRankedModel(m openRouterModel) (AvailableModel, bool) {
	inputPerMtok, ok := tokenPriceToUsdPerMTok(m.Pricing.Prompt)
	if !ok {
		return AvailableModel{}, false
	}
	outputPerMtok, ok := tokenPriceToUsdPerMTok(m.Pricing.Completion)
	if !ok {
		return AvailableModel{}, false
	}
	score := strconv.FormatFloat(*m.Benchmarks.ArtificialAnalysis.IntelligenceIndex, 'f', 1, 64)
	return AvailableModel{
		Info:          model.Info{ID: m.ID, DisplayName: m.Name},
		ContextLength: m.ContextLength,
		InputPerMtok:  &inputPerMtok,
		OutputPerMtok: &outputPerMtok,
		RankScore:     &score,
	}, true
}

// toFullModel converts one vendor model into the wire entry for the full,
// unranked view. Unlike toRankedModel, an unparseable price is left absent
// rather than dropping the whole entry: this view answers "everything the
// vendor named", and a price is only one of the facts about it.
func toFullModel(m openRouterModel) AvailableModel {
	out := AvailableModel{
		Info:          model.Info{ID: m.ID, DisplayName: m.Name},
		ContextLength: m.ContextLength,
	}
	if v, ok := tokenPriceToUsdPerMTok(m.Pricing.Prompt); ok {
		out.InputPerMtok = &v
	}
	if v, ok := tokenPriceToUsdPerMTok(m.Pricing.Completion); ok {
		out.OutputPerMtok = &v
	}
	return out
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
