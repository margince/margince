// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package moneynotation writes a money figure the way a reader's language
// groups digits, so server-written text and the app around it agree.
package moneynotation

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Grouped is the major-unit figure with the language's thousands and decimal
// marks: 12,345.00 in English, 12.345,00 in German and Vietnamese. The digits
// come from values.MajorUnits, so no float ever rounds the amount.
func Grouped(amountMinor int64, currency string, lang textlang.Lang) string {
	whole, fraction, decimal := strings.Cut(values.MajorUnits(amountMinor, currency), ".")
	sign := ""
	if strings.HasPrefix(whole, "-") {
		sign, whole = "-", whole[1:]
	}
	group, point := ",", "."
	if lang == textlang.German || lang == textlang.Vietnamese {
		group, point = ".", ","
	}
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + group + whole[i:]
	}
	if decimal {
		whole += point + fraction
	}
	return sign + whole
}
