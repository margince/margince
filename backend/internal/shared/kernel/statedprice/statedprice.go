// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package statedprice reads whether a customer's own words state a total price
// that is not the deal's value.
//
// It flags, it never corrects. A wrong flag costs a reader one look at the
// quote; a missed one is the status quo. So every doubt answers "no flag".
// Doubts are: no deal value, several sums, a sum in another currency, or a sum
// the words mark as only part of the price.
package statedprice

import (
	"math"
	"regexp"

	"github.com/margince/margince/backend/internal/shared/kernel/specifics"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// partOfThePrice marks a sum as a deposit, an instalment or a price per period
// or unit. Such a sum is a different quantity from the deal's total, so
// comparing the two would flag every subscription.
var partOfThePrice = regexp.MustCompile(`(?i)deposit|down payment|upfront|instal+ments?|` +
	`per[\s-](?:month|year|seat|user|licen[cs]e)|monthly|yearly|annual|\beach\b|/\s?(?:mo|month|yr|year)\b|` +
	`anzahlung|teilzahlung|\braten?\b|monatlich|jährlich|pro (?:monat|jahr|nutzer|lizenz)|` +
	`đặt cọc|trả góp|mỗi tháng|hàng tháng|mỗi năm|hàng năm`)

// Differing returns the total the words state in the deal's currency, in minor
// units, and true when it is not dealMinor. Only one stated sum in that
// currency counts as a total; two are a comparison or a breakdown.
func Differing(words string, dealMinor int64, currency string) (int64, bool) {
	if partOfThePrice.MatchString(words) {
		return 0, false
	}
	var stated []int64
	for _, amount := range specifics.MoneyIn(words) {
		if amount.Currency != currency {
			continue
		}
		minor, ok := values.MinorUnits(amount.Major, currency)
		if !ok || minor > math.MaxInt64/amount.Scale {
			return 0, false
		}
		stated = append(stated, minor*amount.Scale)
	}
	if len(stated) != 1 || stated[0] == dealMinor {
		return 0, false
	}
	return stated[0], true
}
