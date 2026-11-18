// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package personname compares people's names. Dedupe, capture and signature
// reading ask it whether two spellings name one person.
package personname

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Fold casefolds and unaccents (PO-PARAM-JW-2). It need not agree rune for
// rune with Postgres f_unaccent, which only narrows a candidate set. Casefold
// is Unicode full folding, so Straße and STRASSE compare equal. A Caser is
// stateful and not safe for concurrent use, so each call makes its own.
func Fold(s string) string {
	decomposed, _, err := transform.String(norm.NFD, s)
	if err != nil {
		// Decomposition failure means a malformed rune, not an outage:
		// compare what we were given rather than dropping the candidate.
		decomposed = s
	}
	recomposed, _, err := transform.String(norm.NFC, dropAccents(decomposed))
	if err != nil {
		recomposed = decomposed
	}
	return strings.TrimSpace(cases.Fold().String(recomposed))
}

// combiningDiaeresis is the mark that turns Cyrillic "е" into "ё".
const combiningDiaeresis = '\u0308'

// dropAccents removes the combining marks that are ACCENTS and keeps the ones
// that are letters in their own right.
//
// The base letter decides, because NFD puts a mark right after its letter.
// Latin and Greek accents go, so Müller meets Muller. "Mueller" meets it only
// through Digraphs. Hebrew and Arabic vocalization goes too. Any other mark is
// a letter and stays: "เมืองไทย" and "Мойка" keep theirs. Cyrillic's one accent
// pair, "ё" against "е", is spelled out below.
func dropAccents(decomposed string) string {
	var out strings.Builder
	out.Grow(len(decomposed))
	accented, diaeresisAfterYe := false, false
	for _, r := range decomposed {
		switch {
		case unicode.Is(unicode.Mn, r):
			if accented || (diaeresisAfterYe && r == combiningDiaeresis) {
				diaeresisAfterYe = false
				continue
			}
			diaeresisAfterYe = false
		case unicode.Is(unicode.Latin, r), unicode.Is(unicode.Greek, r),
			unicode.Is(unicode.Hebrew, r), unicode.Is(unicode.Arabic, r):
			accented, diaeresisAfterYe = true, false
		case r == 'е' || r == 'Е':
			// Russian writes "ё" and "е" for one letter, and a registry may hold
			// either. So the diaeresis after this one is dropped; NFD has split
			// "ё" into "е" plus its mark.
			//
			// That mark and no other: "ӗ" is "е" with a breve, a letter of
			// Chuvash. "Any mark after е" folded "Ӗнер" and "Енер" into one name.
			diaeresisAfterYe = true
			accented = false
		default:
			accented, diaeresisAfterYe = false, false
		}
		out.WriteRune(r)
	}
	return out.String()
}

// umlautDigraphs writes ä, ö and ü the way German does without them.
var umlautDigraphs = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue")

// Digraphs is the name with its umlauts written as German writes them without
// umlauts, so "Müller" meets "Mueller".
func Digraphs(s string) string { return umlautDigraphs.Replace(norm.NFC.String(s)) }
