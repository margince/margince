// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingrules

import (
	"cmp"
	"reflect"
	"slices"
)

// Loosened names each obligation of above that permits more than below does.
// An empty answer means above is at least as strict as below everywhere.
//
// It asks the fold Strictest applies. above tightens below when folding the two
// changes nothing in above. Fields that name a rule set and bind no send are
// left out, and so is the order of lists the engine reads as sets.
func Loosened(below, above Rules) []string {
	folded := bindingOnly(normalised(stricter(below, above)))
	kept := bindingOnly(normalised(above))
	foldedValue, keptValue := reflect.ValueOf(folded), reflect.ValueOf(kept)
	var out []string
	for i := range foldedValue.NumField() {
		if !reflect.DeepEqual(foldedValue.Field(i).Interface(), keptValue.Field(i).Interface()) {
			out = append(out, foldedValue.Type().Field(i).Name)
		}
	}
	return out
}

// bindingOnly clears the fields that name a rule set and bind nothing.
func bindingOnly(r Rules) Rules {
	r.Jurisdiction, r.Version, r.Instruments = "", 0, nil
	r.Provenance, r.Citations, r.Digest = Provenance{}, nil, ""
	return r
}

// normalised sorts and de-duplicates the lists the engine reads as sets, and
// treats an empty list as an absent one.
func normalised(r Rules) Rules {
	r = clone(r)
	if !r.MarketingMode.OptOut() {
		r.MarketingMode = ""
	}
	for i := range r.Disclosures {
		r.Disclosures[i].Means = setOf(r.Disclosures[i].Means)
	}
	r.Disclosures = slices.CompactFunc(sortedDisclosures(r.Disclosures), func(a, b Disclosure) bool {
		return reflect.DeepEqual(a, b)
	})
	if len(r.Disclosures) == 0 {
		r.Disclosures = nil
	}
	slices.SortFunc(r.MarketingExceptions, func(a, b MarketingException) int { return cmp.Compare(a.Kind, b.Kind) })
	if len(r.MarketingExceptions) == 0 {
		r.MarketingExceptions = nil
	}
	r.ClaimsNeedProof = setOf(r.ClaimsNeedProof)
	r.ConsentRequestFirstFor = setOf(r.ConsentRequestFirstFor)
	return r
}

func sortedDisclosures(in []Disclosure) []Disclosure {
	slices.SortFunc(in, func(a, b Disclosure) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(boolRank(a.MarketingOnly), boolRank(b.MarketingOnly)),
			slices.Compare(a.Means, b.Means))
	})
	return in
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// setOf answers the distinct values of in, sorted, and nil for none.
func setOf[T cmp.Ordered](in []T) []T {
	out := slices.Compact(slices.Sorted(slices.Values(in)))
	if len(out) == 0 {
		return nil
	}
	return out
}
