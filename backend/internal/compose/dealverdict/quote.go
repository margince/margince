// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

// Reading the price out of the customer's own words. The amount Margince sets
// is the one the quote states, never the number the model wrote beside it:
// a mail can carry harmless words while asking the model for any figure.

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// maxQuotedAmount bounds a price in whole units. Larger figures are refused
// rather than trusted.
const maxQuotedAmount = 1_000_000_000_000

// jsSafeMax is the largest minor amount a deal stores and a client reads exact.
const jsSafeMax = 1<<53 - 1

var (
	quotedNumber   = regexp.MustCompile(`\d{1,3}(?:[.,\x{00a0} ]\d{3})+(?:[.,]\d{1,2})?|\d+(?:[.,]\d{1,2})?`)
	quotedCode     = regexp.MustCompile(`\b[A-Z]{3}\b`)
	currencySymbol = map[string]string{"€": "EUR", "$": "USD", "£": "GBP", "¥": "JPY"}
)

// quotedPrice is the one price a quote states, in whole units, and its
// currency. It reports false when the quote holds no number, more than one,
// or no currency.
func quotedPrice(quote string) (int64, string, bool) {
	numbers := quotedNumber.FindAllStringIndex(quote, -1)
	if len(numbers) != 1 {
		return 0, "", false
	}
	start, end := numbers[0][0], numbers[0][1]
	value, ok := parseQuotedNumber(quote[start:end])
	if !ok {
		return 0, "", false
	}
	rest := quote[end:]
	if strings.HasPrefix(strings.ToLower(rest), "k") {
		value *= 1000
		rest = rest[1:]
	}
	currency, ok := quotedCurrency(quote[:start] + " " + rest)
	if !ok || value > maxQuotedAmount {
		return 0, "", false
	}
	return int64(math.Round(value)), currency, true
}

// parseQuotedNumber reads 50,000 / 50.000 / 50 000 / 49,999.50 / 49.999,50.
// A separator followed by exactly three digits groups thousands; one followed
// by one or two digits at the end marks the decimals.
func parseQuotedNumber(text string) (float64, bool) {
	text = strings.ReplaceAll(text, " ", " ")
	decimals := ""
	if i := strings.LastIndexAny(text, ".,"); i >= 0 && len(text)-i-1 <= 2 {
		text, decimals = text[:i], text[i+1:]
	}
	digits := strings.NewReplacer(",", "", ".", "", " ", "").Replace(text)
	if decimals != "" {
		digits += "." + decimals
	}
	value, err := strconv.ParseFloat(digits, 64)
	return value, err == nil
}

// quotedCurrency is the one currency the words around the number name.
func quotedCurrency(around string) (string, bool) {
	found := map[string]bool{}
	for _, code := range quotedCode.FindAllString(around, -1) {
		found[code] = true
	}
	for symbol, code := range currencySymbol {
		if strings.Contains(around, symbol) {
			found[code] = true
		}
	}
	if len(found) != 1 {
		return "", false
	}
	for code := range found {
		return code, true
	}
	return "", false
}

// toMinor multiplies whole units by the currency's scale, and reports false
// when the result leaves the range a deal stores.
func toMinor(whole, scale int64) (int64, bool) {
	if whole <= 0 || whole > maxQuotedAmount || scale <= 0 || whole > jsSafeMax/scale {
		return 0, false
	}
	return whole * scale, true
}
