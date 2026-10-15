// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"
	"maps"
	"slices"
)

// pricedByFor is the provider whose sheet prices provider's unpriced models,
// or "" for one whose unpriced models stay unpriced.
func pricedByFor(provider string) string {
	d, _ := providerByName(provider)
	return d.pricedBy
}

// rateFallbacks are the registry's pricedBy edges as two parallel arrays, the
// shape rateMatch binds: from[i] is priced at to[i]'s rows.
func rateFallbacks() (from, to []string) {
	edges := projectProviders(
		func(d providerDescriptor) string { return d.pricedBy },
		func(d providerDescriptor) bool { return d.pricedBy != "" })
	for _, name := range slices.Sorted(maps.Keys(edges)) {
		from, to = append(from, name), append(to, edges[name])
	}
	return from, to
}

// rateMatch is the rate a model is priced at: the latest ai_model_rate row
// effective on day. The provider's own row wins over the row of the provider
// it is priced by. RateFor and pricedRate both select through it.
//
// provider, model and day are SQL expressions (a placeholder or a column);
// fromArg and toArg are the placeholder numbers bound to rateFallbacks().
func rateMatch(provider, model, day string, fromArg, toArg int) string {
	return fmt.Sprintf(`SELECT mr.* FROM ai_model_rate mr
		WHERE mr.model_id = %[2]s AND mr.effective_date <= %[3]s
		  AND (mr.provider = %[1]s OR mr.provider = (
		    SELECT pf.to_provider FROM unnest($%[4]d::text[], $%[5]d::text[]) AS pf(from_provider, to_provider)
		    WHERE pf.from_provider = %[1]s))
		ORDER BY (mr.provider = %[1]s) DESC, mr.effective_date DESC
		LIMIT 1`, provider, model, day, fromArg, toArg)
}

// pricedRate is the rate the ai_call row aliased alias is priced at: the
// answered model's rate on the call's day, else the binding's. So a served model
// the sheet does not price yet still prices the call. Every SQL reader that
// prices a call joins it LATERAL as r, the alias callCostMicroUSD reads.
func pricedRate(alias string, fromArg, toArg int) string {
	day := alias + ".occurred_at::date"
	return fmt.Sprintf(`SELECT ranked.* FROM (
		  SELECT 0 AS match_rank, answered.* FROM (%[1]s) answered WHERE %[3]s
		  UNION ALL
		  SELECT 1 AS match_rank, bound.* FROM (%[2]s) bound
		) ranked
		ORDER BY ranked.match_rank
		LIMIT 1`,
		rateMatch(alias+".provider", alias+".served_model", day, fromArg, toArg),
		rateMatch(alias+".provider", alias+".model_id", day, fromArg, toArg),
		answeredModelSQL(alias)+" <> "+alias+".model_id")
}
