// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package fieldmask is what a configured mask actually withholds, in one place
// every reader can reach.
//
// A mask names ONE field. Withholding it can require withholding others: a
// currency beside a withheld amount reads as a priced deal with its figure
// missing, and an ARR left standing discloses the size of the deal the mask
// was meant to hide. That grouping is a property of the data rather than of
// whoever is answering, and a surface expanding it for itself expands it less
// — which is how the record read withheld the group while an export, a search
// predicate and an audit diff each matched the configured name alone.
//
// MASKABLE and WITHHELD are kept apart. Maskable is what an administrator may
// configure, and the database offers exactly that set; withheld is what goes
// null when they do, which is wider — partner_attribution travels with the
// partner it describes and is not itself something anyone can configure.
//
// Tier 0, for the reason contactaddress beside it gives: a module never
// imports a sibling, and platform/auth may not import a module at all, so a
// table both of them read can live in neither. stdlib only, which the tier
// requires — the table is strings.
package fieldmask

import (
	"maps"
	"slices"
)

// The wire field names, as a configured mask spells them. They coincide with
// column names today and are not the same vocabulary: renaming a column would
// not rename what an installation's stored mask says.
const (
	DealAmountMinor        = "amount_minor"
	DealExpectedArrMinor   = "expected_arr_minor"
	DealCurrency           = "currency"
	DealCompanyID          = "company_id"
	DealProjectID          = "project_id"
	DealPartnerCompanyID   = "partner_company_id"
	dealPartnerAttribution = "partner_attribution"
)

// Deal is the RBAC object a deal's masks are configured under.
const Deal = "deal"

// withheldWith maps each maskable field to everything withheld when it is
// masked, itself first. A field absent from an object's table is not maskable
// there, and a mask naming one is refused where it is written.
var withheldWith = map[string]map[string][]string{
	Deal: {
		// The money goes as one unit in both directions: masking either figure
		// withholds both and the currency with them.
		DealAmountMinor:      {DealAmountMinor, DealExpectedArrMinor, DealCurrency},
		DealExpectedArrMinor: {DealExpectedArrMinor, DealAmountMinor, DealCurrency},
		DealCurrency:         {DealCurrency},
		DealCompanyID:        {DealCompanyID},
		DealProjectID:        {DealProjectID},
		// "sourced" beside a null partner discloses that SOME partner brought
		// the deal to a reader who may not know which.
		DealPartnerCompanyID: {DealPartnerCompanyID, dealPartnerAttribution},
	},
}

// Maskable is the sorted set of fields an administrator may name on the
// object, and the catalog the database's maskable_field table offers.
func Maskable(object string) []string {
	return slices.Sorted(maps.Keys(withheldWith[object]))
}

// Withheld answers which fields go null when `configured` are masked on the
// object, in a stable order.
//
// This table records what a field drags ALONG WITH IT, so a field with no entry
// withholds itself and nothing else — the answer for every object that groups
// nothing. It is never an allowlist: dropping a name it does not recognise
// would turn a mask on an object nobody thought to list here into a mask that
// withholds nothing, which is the failure this package exists to end rather
// than to relocate.
func Withheld(object string, configured []string) []string {
	table := withheldWith[object]
	var out []string
	for _, field := range configured {
		group, grouped := table[field]
		if !grouped {
			group = []string{field}
		}
		for _, withheld := range group {
			if !slices.Contains(out, withheld) {
				out = append(out, withheld)
			}
		}
	}
	return out
}

// Covers reports whether the object's `field` is withheld when `configured`
// are masked — the question a sort, a filter or a search predicate asks before
// it compiles, because ordering or filtering by a value is reading it.
func Covers(object string, configured []string, field string) bool {
	return slices.Contains(Withheld(object, configured), field)
}
