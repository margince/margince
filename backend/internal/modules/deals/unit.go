// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"maps"
	"strings"
)

// defaultUnit is the unit key a product or line gets when nobody named one.
// The client shows each key in its reader's language.
const defaultUnit = "piece"

// unitSpellings maps a unit's spelling, lower-cased, to its key. Each key spells
// itself. The client reads rows written while the unit was free text through
// its own copy of this table, frontend/src/features/deals/productunit.tsx, and
// backend/gates/frontendunitspellings_test.go holds the two equal.
var unitSpellings = map[string]string{
	"piece": "piece", "pieces": "piece", "unit": "piece", "units": "piece",
	"pc": "piece", "pcs": "piece", "each": "piece", "item": "piece", "items": "piece",
	"stück": "piece", "stueck": "piece", "stk": "piece", "cái": "piece",
	"hour": "hour", "hours": "hour", "h": "hour", "hr": "hour", "hrs": "hour",
	"stunde": "hour", "stunden": "hour", "std": "hour", "giờ": "hour",
	"day": "day", "days": "day", "tag": "day", "tage": "day",
	"personentag": "day", "personentage": "day", "ngày": "day",
	"month": "month", "months": "month", "monat": "month", "monate": "month", "tháng": "month",
	"flat_fee": "flat_fee", "flat fee": "flat_fee", "flat rate": "flat_fee", "lump sum": "flat_fee",
	"pauschal": "flat_fee", "pauschale": "flat_fee", "festpreis": "flat_fee", "trọn gói": "flat_fee",
}

// UnitSpellings is the spelling-to-key table, for the gate that holds the
// client's copy equal to it.
func UnitSpellings() map[string]string {
	return maps.Clone(unitSpellings)
}

// canonicalUnit stores a known spelling as its key, so "Stunde" sent by an
// agent reads as an hour in every language. Unknown text stays as typed.
func canonicalUnit(trimmed string) string {
	if key, ok := unitSpellings[strings.ToLower(trimmed)]; ok {
		return key
	}
	return trimmed
}
