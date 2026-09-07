// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package specifics extracts the checkable facts a sentence states — its dates,
// numbers, money amounts and percentages — so generated prose can be held
// against the record it cites.
//
// A citation is a POINTER. Matching one proves a model named a real row, never
// that the sentence says what the row says: "she asked for times on 2 June" and
// "they want 40% off in euros" both cite correctly and are both wrong if the
// row says otherwise. What survives the check is the READING — asked, still
// open, nobody replied — which is the judgment a brief exists to make and which
// the claim's own nature label governs separately.
//
// What is checkable is deliberately narrow: DATES, PERCENTAGES and the
// CURRENCY a figure is in. Counts and money magnitudes are not, because they
// are routinely derived rather than read — three open deals, fourteen days, a
// pipeline worth the sum of its rows — and the payload holds the parts. See
// numbers.go, which states the case; the product's own deterministic floor is
// the proof.
//
// The comparison is on canonical VALUES, never on substrings, because the
// product writes in the reader's language: 2 June, 2. Juni, 2026-06-02 and
// 02/06/2026 are one date, and €1.200,00 and 1200.00 are one amount. A
// substring rule would drop true sentences by the hundred in every locale but
// the one it happened to be written against — and dropping true sentences is
// the failure this check must not trade for the one it prevents.
//
// Proper names are deliberately NOT extracted here. A capitalised token is a
// name in English and an ordinary noun in German, and no rule this package
// could apply tells "Firma" from "Fischer" without a dictionary — so the name
// half is the caller's, against its own record vocabulary, where a name is
// known rather than guessed.
package specifics

import "strings"

// A Specific is one checkable fact, as written and as compared.
type Specific struct {
	// Text is the fact in the sentence's own words, so a rejection can say
	// which one failed rather than only that something did.
	Text string
	// Keys are the canonical forms it may match under. More than one when the
	// written form is genuinely ambiguous — 1.200 is twelve hundred to a German
	// reader and one-point-two to an English one, and a checker that picked
	// either would be wrong half the time.
	Keys []string
}

// Missing returns the specifics `claim` states that `source` does not.
//
// An empty answer is the sentence being extractive: every date, amount and
// figure it names is in the text it was written from. The connective prose is
// deliberately untouched — it is where the reading lives.
func Missing(claim, source string) []Specific {
	stated := In(claim)
	if len(stated) == 0 {
		return nil
	}
	available := sourceKeys(source)
	var missing []Specific
	for _, fact := range stated {
		if !anyKnown(fact.Keys, available) {
			missing = append(missing, fact)
		}
	}
	return missing
}

// In returns what a sentence STATES: the most specific reading of each fact,
// plus the alternatives an ambiguous written form genuinely has.
//
// Dates are taken first and blanked out of what the number scan then reads.
// Without that, 2026-06-02 states the numbers 2026, 6 and 2 as well as the day
// it names, and a sentence would have to find a stray "2026" in its source to
// survive naming a date that was already there.
func In(text string) []Specific {
	dates, rest := datesIn(text)
	return append(dates, numbersIn(rest, true)...)
}

// sourceKeys collects the canonical forms a source text OFFERS, which is
// broader than what the same text would STATE: a source dated 2026-06-02 answers a
// sentence that says "2 June" and one that says "in June", because both are
// true of it. The claim side stays narrow, so the asymmetry only ever admits a
// sentence that says LESS than its source, never more.
func sourceKeys(text string) map[string]bool {
	keys := make(map[string]bool)
	dates, rest := datesIn(text)
	for _, date := range dates {
		for _, key := range date.Keys {
			keys[key] = true
			for _, coarser := range lessSpecific(key) {
				keys[coarser] = true
			}
		}
	}
	for _, number := range numbersIn(rest, false) {
		for _, key := range number.Keys {
			keys[key] = true
		}
	}
	return keys
}

func anyKnown(keys []string, known map[string]bool) bool {
	for _, key := range keys {
		if known[key] {
			return true
		}
	}
	return false
}

// Texts renders the missing specifics for a message a person reads.
func Texts(missing []Specific) string {
	words := make([]string, 0, len(missing))
	for _, fact := range missing {
		words = append(words, fact.Text)
	}
	return strings.Join(words, ", ")
}
