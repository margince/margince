// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"maps"
	"strings"
)

// The unit keys. The client shows each one in its reader's language.
const (
	unitPiece   = "piece"
	unitHour    = "hour"
	unitDay     = "day"
	unitMonth   = "month"
	unitFlatFee = "flat_fee"
)

// defaultUnit is the unit key a product or line gets when nobody named one.
const defaultUnit = unitPiece

// unitSpellings maps a unit's spelling, lower-cased, to its key. Each key spells
// itself. The client reads rows written while the unit was free text through
// its own copy of this table, frontend/src/features/deals/productunit.tsx, and
// backend/gates/frontendunitspellings_test.go holds the two equal.
var unitSpellings = map[string]string{
	unitPiece: unitPiece, "pieces": unitPiece, "unit": unitPiece, "units": unitPiece,
	"pc": unitPiece, "pcs": unitPiece, "each": unitPiece, "item": unitPiece, "items": unitPiece,
	"stück": unitPiece, "stueck": unitPiece, "stk": unitPiece, "cái": unitPiece,
	unitHour: unitHour, "hours": unitHour, "h": unitHour, "hr": unitHour, "hrs": unitHour,
	"stunde": unitHour, "stunden": unitHour, "std": unitHour, "giờ": unitHour,
	unitDay: unitDay, "days": unitDay, "tag": unitDay, "tage": unitDay,
	"personentag": unitDay, "personentage": unitDay, "ngày": unitDay,
	unitMonth: unitMonth, "months": unitMonth, "monat": unitMonth, "monate": unitMonth, "tháng": unitMonth,
	unitFlatFee: unitFlatFee, "flat fee": unitFlatFee, "flat rate": unitFlatFee, "lump sum": unitFlatFee,
	"pauschal": unitFlatFee, "pauschale": unitFlatFee, "festpreis": unitFlatFee, "trọn gói": unitFlatFee,
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
