// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

// Reading the price out of the customer's own words. The amount Margince sets
// is the one the quote states, never the number the model wrote beside it.
// A mail can carry harmless words while asking the model for any figure.

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxQuotedAmount bounds a price in whole units. Larger figures are refused
// rather than trusted.
const maxQuotedAmount = 1_000_000_000_000

// jsSafeMax is the largest minor amount a deal stores and a client reads exact.
const jsSafeMax = 1<<53 - 1

var (
	quotedNumber = regexp.MustCompile(`\d{1,3}(?:[.,\x{00a0} ]\d{3})+(?:[.,]\d{1,2})?|\d+(?:[.,]\d{1,2})?`)
	quotedCode   = regexp.MustCompile(`\b[A-Z]{3}\b`)
	// quotedCodeAtEnd and quotedCodeAtStart find a code touching a number.
	quotedCodeAtEnd   = regexp.MustCompile(`\b[A-Z]{3}$`)
	quotedCodeAtStart = regexp.MustCompile(`^[A-Z]{3}\b`)
	currencySymbol    = map[string]string{"€": "EUR", "$": "USD", "£": "GBP", "¥": "JPY"}
)

// quotedPrice is the one price a quote states, in exact minor units, and its
// currency. It reports false when the quote holds no number, or no currency,
// or more than one currency. It also reports false for several numbers unless
// only one of them has its currency written next to it.
func quotedPrice(quote string) (int64, string, bool) {
	start, end, ok := priceNumber(quote)
	if !ok {
		return 0, "", false
	}
	decimal := plainDecimal(quote[start:end])
	rest := quote[end:]
	if strings.HasPrefix(strings.ToLower(rest), "k") {
		decimal = thousands(decimal)
		rest = rest[1:]
	}
	currency, ok := quotedCurrency(quote[:start] + " " + rest)
	if !ok {
		return 0, "", false
	}
	minor, ok := values.MinorUnits(decimal, currency)
	if !ok || minor > maxQuotedAmount*values.MinorUnitScale(currency) || minor > jsSafeMax {
		return 0, "", false
	}
	return minor, currency, true
}

// priceNumber is where the price's number stands in the quote. That is its
// only number, or of several the one a currency touches: "60,000 USD for 2027".
func priceNumber(quote string) (int, int, bool) {
	numbers := quotedNumber.FindAllStringIndex(quote, -1)
	if len(numbers) == 1 {
		return numbers[0][0], numbers[0][1], true
	}
	var priced [][]int
	for _, number := range numbers {
		if touchesCurrency(quote[:number[0]], quote[number[1]:]) {
			priced = append(priced, number)
		}
	}
	if len(priced) != 1 {
		return 0, 0, false
	}
	return priced[0][0], priced[0][1], true
}

// touchesCurrency reports whether a currency is written right before or right
// after a number, spaces and a thousands "k" aside.
func touchesCurrency(before, after string) bool {
	before = strings.TrimRight(before, " \u00a0")
	after = strings.TrimLeft(after, " \u00a0")
	if strings.HasPrefix(strings.ToLower(after), "k") {
		after = strings.TrimLeft(after[1:], " \u00a0")
	}
	for symbol := range currencySymbol {
		if strings.HasSuffix(before, symbol) || strings.HasPrefix(after, symbol) {
			return true
		}
	}
	return quotedCodeAtEnd.MatchString(before) || quotedCodeAtStart.MatchString(after)
}

// plainDecimal turns 50,000 / 50.000 / 50 000 / 49,999.50 / 49.999,50 into a
// plain decimal. A separator followed by three digits groups thousands; one
// followed by one or two digits at the end marks the decimals.
func plainDecimal(text string) string {
	text = strings.ReplaceAll(text, " ", " ")
	decimals := ""
	if i := strings.LastIndexAny(text, ".,"); i >= 0 && len(text)-i-1 <= 2 {
		text, decimals = text[:i], text[i+1:]
	}
	digits := strings.NewReplacer(",", "", ".", "", " ", "").Replace(text)
	if decimals != "" {
		digits += "." + decimals
	}
	return digits
}

// thousands is a plain decimal times a thousand, written by moving the point.
func thousands(decimal string) string {
	whole, frac, _ := strings.Cut(decimal, ".")
	frac += "000"
	return strings.TrimLeft(whole+frac[:3], "0") + strings.TrimRight("."+frac[3:], ".0")
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

// QuotedPrice is quotedPrice for a caller outside the verdict: the one price a
// quote states, in exact minor units, and its currency.
func QuotedPrice(quote string) (int64, string, bool) { return quotedPrice(quote) }
