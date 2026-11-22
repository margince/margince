// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

import (
	"math/big"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Amount is one sum of money a sentence states, with its currency named.
type Amount struct {
	// Currency is the ISO code the written mark names.
	Currency string
	// Major is the figure as a plain decimal in major units, before Scale.
	Major string
	// Scale is 1, or 1 000 or 1 000 000 for a figure written "45k" or "2 Mio.".
	Scale int64
	// Negative says the sum was written with a minus sign.
	Negative bool
	// Text is the sum as written, for a message a reader reads.
	Text string
	// Keys are the (currency, value) pairs it may be. More than one when the
	// written separator is ambiguous in a currency with three minor digits.
	Keys []string
}

// moneyFigure is one sum or one range of sums with a currency mark on either
// side. A word mark needs a word boundary, so "eur" never matches inside
// "Europe" and "m" never inside "more"; a symbol carries its own. The pattern
// captures a sign before the figure or its mark. It also captures the second
// end of a range, which shares the currency of the first.
var moneyFigure = regexp.MustCompile(`(?i)([-−])?(?:(?P<sym1>` + symbolMarks + `)|\b(?P<word1>` + wordMarks + `)\b)?\s?` +
	`(?P<num1>` + moneyNumber + `)(?:\s?(?P<scale1>` + moneyScales + `)(?:\.|\b))?` +
	`(?:(?:\s?[-–—]\s?|\s(?:to|bis|đến)\s)(?P<num2>` + moneyNumber + `)(?:\s?(?P<scale2>` + moneyScales + `)(?:\.|\b))?)?` +
	`(?:\s?(?:(?P<sym2>` + symbolMarks + `)|\b(?P<word2>` + wordMarks + `)\b))?`)

const (
	// moneyMaxInt and moneyMaxFrac bound a sum the parser will read.
	moneyMaxInt, moneyMaxFrac = 18, 3
	moneyNumber               = `\d{1,3}(?:[.,' ]\d{3})+(?:[.,]\d{1,3})?|\d+(?:[.,]\d{1,3})?`
	moneyScales               = `k|m|mio|tsd|thousand|million|millions|millionen`
)

var symbolMarks, wordMarks = markAlternations()

// markAlternations is the symbols and words a sum's currency is written as.
// The words are the colloquial marks of currencies.go. Every assigned ISO-4217
// code is also a mark, in capitals only: a lower-case "all" or "try" next to a
// number is a word.
func markAlternations() (symbols, words string) {
	bySymbol, byWord := map[string]int{}, map[string]int{}
	for mark := range currencies {
		first, _ := utf8.DecodeRuneInString(mark)
		if unicode.IsLetter(first) {
			byWord[mark] = 0
		} else {
			bySymbol[mark] = 0
		}
	}
	codes := map[string]int{}
	for _, code := range values.AssignedCurrencies() {
		codes[code] = 0
	}
	return alternation(bySymbol), alternation(byWord) + `|(?-i:` + alternation(codes) + `)`
}

var scales = map[string]int64{
	"k": 1_000, "tsd": 1_000, "thousand": 1_000,
	"m": 1_000_000, "mio": 1_000_000, "million": 1_000_000, "millions": 1_000_000, "millionen": 1_000_000,
}

// MoneyIn returns the sums a text states with a currency named, each end of a
// range as its own sum. A figure with no currency is not money to this reader:
// "40 seats" and "40 EUR" share a digit and nothing else. A sum written with a
// minus sign is negative, and never equals the same sum without one.
func MoneyIn(text string) []Amount {
	var found []Amount
	for _, at := range moneyFigure.FindAllStringSubmatchIndex(text, -1) {
		group := func(name string) string {
			i := moneyFigure.SubexpIndex(name) * 2
			if at[i] < 0 {
				return ""
			}
			return text[at[i]:at[i+1]]
		}
		code := moneyCurrency(firstNonEmpty(group("sym1"), group("word1"), group("sym2"), group("word2")))
		if code == "" {
			continue
		}
		negative := at[2] >= 0 && !followsWordCharacter(text, at[0])
		scale1, scale2 := scaleOf(group("scale1")), scaleOf(group("scale2"))
		if group("num2") != "" && scale1 == 1 {
			scale1 = scale2
		}
		found = append(found, amountOf(code, group("num1"), scale1, negative))
		if second := group("num2"); second != "" {
			found = append(found, amountOf(code, second, scale2, negative))
		}
	}
	return found
}

// followsWordCharacter says a sign at `at` is a hyphen inside a word or a
// number, as in "Q3-50,000 EUR", and so not a minus.
func followsWordCharacter(text string, at int) bool {
	before, _ := utf8.DecodeLastRuneInString(text[:at])
	return unicode.IsLetter(before) || unicode.IsDigit(before)
}

// moneyCurrency is the code a written mark names: a colloquial mark, or an
// assigned code in capitals.
func moneyCurrency(mark string) string {
	mark = strings.TrimSpace(mark)
	if code := currencyCode(mark); code != "" {
		return code
	}
	if values.AssignableCurrency(mark) {
		return mark
	}
	return ""
}

func scaleOf(mark string) int64 {
	if mark == "" {
		return 1
	}
	return scales[strings.ToLower(mark)]
}

func amountOf(code, raw string, scale int64, negative bool) Amount {
	sign := ""
	if negative {
		sign = "-"
	}
	amount := Amount{Currency: code, Scale: scale, Negative: negative, Text: strings.TrimSpace(code + " " + sign + raw)}
	readings := moneyReadings(raw, values.MinorUnitDigits(code))
	amount.Major = readings[len(readings)-1]
	for _, reading := range readings {
		value, ok := plainRat(reading)
		if !ok {
			continue
		}
		value.Mul(value, new(big.Rat).SetInt64(scale))
		amount.Keys = append(amount.Keys, code+":"+sign+strings.TrimRight(strings.TrimRight(value.FloatString(6), "0"), "."))
	}
	return amount
}

// plainRat parses a plain decimal only. Exponent and fraction forms would let
// big.Rat size the value by the digits of the text.
func plainRat(reading string) (*big.Rat, bool) {
	if !values.PlainDecimal(reading, moneyMaxInt, moneyMaxFrac) {
		return nil, false
	}
	return new(big.Rat).SetString(reading)
}

// moneyReadings reads a written sum as plain decimals. A single separator
// before three digits groups thousands. In a currency with three minor digits
// it is also a decimal point: KWD 12.345 is twelve dinar.
func moneyReadings(raw string, minorDigits int) []string {
	digits := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\'' {
			return -1
		}
		return r
	}, raw)
	if strings.Contains(digits, ".") && strings.Contains(digits, ",") {
		return []string{mixed(digits)}
	}
	at, last := strings.IndexAny(digits, ".,"), strings.LastIndexAny(digits, ".,")
	if at < 0 {
		return []string{digits}
	}
	grouped, decimal := strings.Map(dropSeparators, digits), digits[:at]+"."+digits[at+1:]
	switch {
	case at != last:
		return []string{grouped}
	case len(digits)-last-1 != 3:
		return []string{decimal}
	case minorDigits == 3:
		return []string{grouped, decimal}
	}
	return []string{grouped}
}

// MoneyMissing returns the sums `claim` states that `source` does not state as
// the same sum in the same currency. Missing leaves money magnitudes alone,
// because a count or a total is routinely derived. A caller whose sources hold
// every sum the product calculated can refuse any other.
//
// The sums a source states are the ones written with a currency. A count of 3
// never answers "EUR 3". A sum in one currency never answers another.
func MoneyMissing(claim, source string) []Specific {
	var available map[string]bool
	var missing []Specific
	for _, amount := range MoneyIn(claim) {
		if available == nil {
			available = make(map[string]bool)
			for _, stated := range MoneyIn(source) {
				for _, key := range stated.Keys {
					available[key] = true
				}
			}
		}
		if !anyKnown(amount.Keys, available) {
			missing = append(missing, Specific{Text: amount.Text, Keys: amount.Keys})
		}
	}
	return missing
}

func firstNonEmpty(marks ...string) string {
	for _, v := range marks {
		if v != "" {
			return v
		}
	}
	return ""
}
