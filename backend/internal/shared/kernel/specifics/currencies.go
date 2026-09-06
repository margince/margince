// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package specifics

// The currencies a sentence can name, and the one code each of them is.
//
// A figure's currency is a fact of its own: "they want 40,000 dollars" on a
// deal priced in euros is wrong about the only thing the number was for. The
// source says EUR because that is what the assembler puts in the payload, and
// the sentence says € or "Euro" because that is what a reader reads, so both
// sides fold to the code.

import "strings"

// currencyMarks is each code and the marks a reader or a payload writes it as.
// Deliberately short: the currencies this product's own payloads and readers
// actually use, rather than the whole ISO table, because a mark nobody writes
// is a pattern alternative that only costs matching time.
//
// Keyed by CODE rather than by mark, which keeps a code from being repeated
// down a column of marks, and inverted below into the lookup matching needs.
var currencyMarks = map[string][]string{
	"EUR": {"€", "eur", "euro", "euros"},
	"USD": {"$", "usd", "dollar", "dollars"},
	"GBP": {"£", "gbp", "pound", "pounds"},
	"CHF": {"chf", "franken", "francs"},
	"VND": {"₫", "vnd", "đồng", "dong"},
	"JPY": {"¥", "jpy", "yen"},
}

// currencies is currencyMarks read the way a match needs it: mark to code.
var currencies = byMark(currencyMarks)

func byMark(codes map[string][]string) map[string]string {
	marks := make(map[string]string)
	for code, written := range codes {
		for _, mark := range written {
			marks[mark] = code
		}
	}
	return marks
}

var currencyAlternatives = alternationOf(currencies)

func alternationOf(vocabulary map[string]string) string {
	numbered := make(map[string]int, len(vocabulary))
	for word := range vocabulary {
		numbered[word] = 0
	}
	return alternation(numbered)
}

// currencyWord is what a mark must be surrounded by to count. A symbol carries
// its own boundary and a word does not: matching "eur" inside "Europe" would
// put a currency in every sentence about the continent.

// currencyCode is the code a written mark names, empty when it names none.
func currencyCode(mark string) string {
	return currencies[strings.ToLower(strings.TrimSpace(mark))]
}
