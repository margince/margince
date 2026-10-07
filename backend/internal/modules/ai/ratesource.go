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

// rateMatch is the rate a call is priced at: the latest ai_model_rate row
// effective on day for the model, the provider's own row when one is
// effective, else the row of the provider it is priced by. RateFor, the cost
// report and a run's transparency select through it, so they price a call
// alike.
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
