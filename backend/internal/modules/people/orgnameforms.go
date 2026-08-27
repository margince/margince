// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Org-name matching for the markets that write the legal form BEFORE the name.
//
// PO-PARAM-1 strips a form that TRAILS the name, because that is where English
// and German put it. Much of the world puts it in front — "CÔNG TY CỔ PHẦN SỮA
// VIỆT NAM" is Vinamilk and the first three words are "joint-stock company" — so
// every company in such a market begins with the same run of words, and
// Jaro-Winkler's prefix boost read that as shared identity. Measured before this
// existed, two unrelated companies scored 0.76 to 0.91 against each other.
//
// A PHRASE, NEVER A WORD. The folded syllables of a legal form are also brand
// syllables: "cổ" and "cỏ" both fold to "co", and "Cỏ May" is a rice company. So
// a word is only removed as part of the whole phrase, at the front of the name.
//
// AND A SHORT LATIN FORM NEEDS A SECOND OPINION. "PT Solutions Physical
// Therapy", "AO World" and "AS Roma" are real companies whose first word is
// another market's legal form — see corroboratedMarket.
//
// NOT a second normalizer for the org-name KEY: NormalizeOrgName is unchanged
// and still produces the exact and grouping keys, so no stored key moves. This
// one is consulted only where two names are COMPARED.

import (
	"strings"
	"unicode"
)

// foldDStroke maps đ to d. U+0111 has no canonical decomposition, so the NFD
// pass in normalizeName leaves it while Postgres f_unaccent maps it — without
// this, "ĐẦU TƯ" folds to "đau tu" in Go and "dau tu" in the database.
//
// Not inside normalizeName, which is pinned as the similarity metric's input
// (PO-PARAM-JW-2) and shared with person and lead matching.
func foldDStroke(s string) string {
	if !strings.ContainsAny(s, "đĐ") {
		return s
	}
	return strings.NewReplacer("đ", "d", "Đ", "D").Replace(s)
}

// stripLeadingMarkers removes legal forms from the FRONT of a name, longest
// first, for as long as one matches — the forms stack ("TỔNG CÔNG TY CỔ PHẦN"
// carries two), and taking "cong ty" off "cong ty co phan x" first would leave
// the remains of a longer form behind.
//
// An AMBIGUOUS marker is skipped unless the caller found corroboration.
func stripLeadingMarkers(fields []string, markers []orgFormMarker, corroborated bool) []string {
	for {
		longest := 0
		for _, marker := range markers {
			if marker.ambiguous && !corroborated {
				continue
			}
			if len(marker.tokens) > longest && len(marker.tokens) <= len(fields) &&
				opensWith(fields, marker.tokens) {
				longest = len(marker.tokens)
			}
		}
		if longest == 0 {
			return fields
		}
		fields = fields[longest:]
	}
}

// nameWordSeparators split a name into words, for the tables here and the gate
// that compares what is left.
//
// PUNCTUATION SEPARATES, as a class rather than a list: names arrive punctuated
// every way a writer can manage, and a fused "acme-group" matches nothing. An
// earlier version named six ASCII characters and missed the period and the en
// dash.
//
// A COMBINING MARK DOES NOT. Thai, Lao, Khmer and the Indic scripts write vowels
// as marks, so a letters-and-digits rule cut "เมืองไทย" into four fragments.
// Those scripts have no spaces either, so a Thai name is correctly one word.
//
// NOT done inside NormalizeOrgName, which also produces exact grouping keys —
// splitting there would make two DIFFERENT names equal as keys.
func nameWordSeparators(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r)
}

// stripTrailingWords removes a market's own trailing forms, for the bracketing
// markets where the brand sits between two halves of one legal form.
//
// Never down to nothing: a name that is only its legal form keeps the last word,
// the same rule NormalizeOrgName follows when a suffix IS the company.
func stripTrailingWords(fields []string, suffixes []string) []string {
	for len(fields) > 1 {
		last := fields[len(fields)-1]
		matched := false
		for _, suffix := range suffixes {
			if last == suffix {
				matched = true
				break
			}
		}
		if !matched {
			return fields
		}
		fields = fields[:len(fields)-1]
	}
	return fields
}

// stripTrailingRuns removes a multi-word trailing form, longest first, for as
// long as one matches. Never down to nothing, the same rule the other strips
// follow.
func stripTrailingRuns(fields []string, runs [][]string) []string {
	for {
		longest := 0
		for _, run := range runs {
			if len(run) > longest && len(run) < len(fields) &&
				closesWithRun(fields, run) {
				longest = len(run)
			}
		}
		if longest == 0 {
			return fields
		}
		fields = fields[:len(fields)-longest]
	}
}

// closesWithRun answers whether the name ends in exactly this run of words.
func closesWithRun(fields, run []string) bool {
	tail := fields[len(fields)-len(run):]
	for i, word := range run {
		if tail[i] != word {
			return false
		}
	}
	return true
}

// opensWith answers whether the name opens with exactly this form.
func opensWith(fields, tokens []string) bool {
	for i, word := range tokens {
		if strings.Trim(fields[i], ".") != word {
			return false
		}
	}
	return true
}

// corroboratedMarket answers whether a name that OPENS with an ambiguous marker
// really belongs to that market. "PT Astra International Tbk" is Indonesian and
// "PT Solutions Physical Therapy" is American, and the position of "PT" does not
// distinguish them.
//
// Two things count, both properties of the name itself: the market's own
// bracketing form closes it ("Tbk"), or the rest is not Latin script. When
// neither holds the marker stays, which is the safer error — a name wrongly
// stripped matches every company in its sector.
func corroboratedMarket(raw string, fields []string, market marketForms) bool {
	if len(fields) < 2 {
		return false
	}
	for _, closer := range market.closers {
		// Asked of the RAW name, because the suffix strip in NormalizeOrgName
		// has already removed the closer from `fields` by the time this runs —
		// the evidence would be gone exactly when it is needed.
		if closesWith(raw, closer) {
			return true
		}
	}
	return !isLatinScript(fields[1:])
}

// closesWith answers whether a name ends in this word.
func closesWith(raw, word string) bool {
	words := strings.FieldsFunc(normalizeName(raw), nameWordSeparators)
	return len(words) > 0 && words[len(words)-1] == word
}

// isLatinScript answers whether these words are written in the Latin alphabet.
//
// Digits and punctuation say nothing either way, so a name of nothing but those
// counts as Latin: it carries no evidence of another market, which is what the
// caller is asking about.
func isLatinScript(fields []string) bool {
	for _, field := range fields {
		for _, r := range field {
			if !unicode.IsLetter(r) {
				continue
			}
			if !unicode.Is(unicode.Latin, r) {
				return false
			}
		}
	}
	return true
}

// orgNameForMatching is the name reduced to what could identify a company.
//
// IT MAY RETURN EMPTY, which is an answer: a name of nothing but a legal form and
// trade words has not said WHICH company it is. Callers treat empty as no
// evidence, never as a wildcard — which is the one thing NormalizeOrgName may
// not do (namesim.go).
func orgNameForMatching(s string) string {
	name, _ := matchingFormOf(s)
	return name
}

// matchingFormOf is orgNameForMatching plus the market the name declared, which
// the gate needs to know whether one shared word is evidence.
//
// THE TRADE VOCABULARY IS ONLY STRIPPED FROM A NAME THAT DECLARED ITS MARKET.
// Vietnam's abbreviations are two letters — "tm", "dv", "dt" — and two letters
// mean something else elsewhere: "VA Trading" and "DT Robotics" are companies
// whose first word this table would otherwise eat.
func matchingFormOf(s string) (string, *marketForms) {
	// The scripts with no word boundaries are answered first, by substring
	// (orgnamecjk.go). Splitting them into words yields the whole name as one
	// token, so every path below would find nothing to strip and nothing to
	// compare.
	if name, isCJK := cjkNameForMatching(s); isCJK {
		return name, nil
	}
	fields := strings.FieldsFunc(NormalizeOrgName(foldDStroke(s)), nameWordSeparators)
	for i := range prefixMarkets {
		market := &prefixMarkets[i]
		stripped := stripLeadingMarkers(fields, market.prefixes,
			corroboratedMarket(s, fields, *market))
		if len(stripped) < len(fields) {
			stripped = stripLeadingMarkers(stripped, market.continuations, true)
			stripped = stripLeadingMarkers(stripped, market.fillers, true)
		}
		// The trailing forms are consulted whether or not a leading one
		// matched, because most of them do not come in pairs: "Alfa Sp. z o.o."
		// carries the Polish form at the end and nothing at the front, and a
		// name that reached the suffix strip only through a prefix would keep
		// it and share four trailing words with every other Polish company.
		stripped = stripTrailingWords(stripped, market.suffixes)
		stripped = stripTrailingRuns(stripped, market.suffixRuns)
		if len(stripped) < len(fields) {
			return strings.Join(stripped, " "), market
		}
	}
	return strings.Join(fields, " "), nil
}
