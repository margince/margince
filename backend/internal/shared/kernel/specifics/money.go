// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Amount is one sum of money a sentence states with its currency named.
type Amount struct {
	// Currency is the ISO code the written mark names.
	Currency string
	// Major is the figure as a plain decimal in major units, before Scale.
	Major string
	// Scale is 1, or 1 000 or 1 000 000 for a figure written "45k" or "2 Mio.".
	Scale int64
}

// moneyFigure is a figure with a currency mark on one side. A word mark needs a
// word boundary, so "eur" never matches inside "Europe" and "m" never inside
// "more"; a symbol carries its own.
var moneyFigure = regexp.MustCompile(`(?i)(?:(` + symbolMarks + `)|\b(` + wordMarks + `)\b)?\s?` +
	`(\d{1,3}(?:[.,' ]\d{3})+(?:[.,]\d{1,2})?|\d+(?:[.,]\d{1,2})?)` +
	`(?:\s?(k|m|mio|tsd|thousand|million|millions|millionen)(?:\.|\b))?` +
	`(?:\s?(?:(` + symbolMarks + `)|(` + wordMarks + `)\b))?`)

var symbolMarks, wordMarks = markAlternations()

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
	return alternation(bySymbol), alternation(byWord)
}

var scales = map[string]int64{
	"k": 1_000, "tsd": 1_000, "thousand": 1_000,
	"m": 1_000_000, "mio": 1_000_000, "million": 1_000_000, "millions": 1_000_000, "millionen": 1_000_000,
}

// MoneyIn returns the sums a text states with a currency named. A figure with
// no currency is not money to this reader: "40 seats" and "40 EUR" share a
// digit and nothing else.
func MoneyIn(text string) []Amount {
	var found []Amount
	for _, m := range moneyFigure.FindAllStringSubmatch(text, -1) {
		code := currencyCode(firstNonEmpty(m[1], m[2], m[5], m[6]))
		if code == "" {
			continue
		}
		scale := int64(1)
		if m[4] != "" {
			scale = scales[strings.ToLower(m[4])]
		}
		found = append(found, Amount{Currency: code, Major: moneyMajor(m[3]), Scale: scale})
	}
	return found
}

// moneyMajor reads a written sum as one plain decimal. Unlike a bare figure,
// a sum is not ambiguous: no currency this reader names has three minor
// digits, so one separator before three final digits groups thousands.
func moneyMajor(raw string) string {
	digits := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\'' {
			return -1
		}
		return r
	}, raw)
	if strings.Contains(digits, ".") && strings.Contains(digits, ",") {
		return mixed(digits)
	}
	at, last := strings.IndexAny(digits, ".,"), strings.LastIndexAny(digits, ".,")
	if at < 0 || at != last || len(digits)-last-1 == 3 {
		return strings.Map(dropSeparators, digits)
	}
	return digits[:at] + "." + digits[at+1:]
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
