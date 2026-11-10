// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "strings"

// The tool surface spells money as `<name>_minor` beside the currency it is
// counted in, and writes a readable `<name>_display` next to it. The surface's
// answer and the schema the generator publishes for it read this one rule. So
// a closed schema admits every display the surface adds.

// MinorAmountSuffix ends the name of an amount counted in minor units.
const MinorAmountSuffix = "_minor"

// AmountCurrency names the currency an object's own amounts are counted in, and
// ConvertedCurrency the one a forecast converted its figures to.
const (
	AmountCurrency    = "currency"
	ConvertedCurrency = "base_currency"
)

// AmountCurrencyMembers name the currency an object's amounts are counted in.
func AmountCurrencyMembers() []string { return []string{AmountCurrency, ConvertedCurrency} }

// DisplayMemberOf is the readable member written beside the minor-unit amount
// member, and whether member is one.
func DisplayMemberOf(member string) (string, bool) {
	base, minor := strings.CutSuffix(member, MinorAmountSuffix)
	if !minor || base == "" {
		return "", false
	}
	return base + "_display", true
}
