// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Org-name matching for the markets that write the legal form BEFORE the name.
//
// WHY THIS EXISTS. PO-PARAM-1 strips a legal form that TRAILS the name — "Acme
// Inc", "Acme GmbH" — because that is where English and German put it. Much of
// the world puts it in front: "CÔNG TY CỔ PHẦN SỮA VIỆT NAM" is Vinamilk and the
// first three words are "joint-stock company"; so are "PT Astra International",
// "ООО Ромашка", "UAB Vilniaus Duona". Every company in such a market therefore
// begins with the same run of words, and a character metric reads that as shared
// identity — Jaro-Winkler boosts a shared PREFIX specifically. Measured before
// this existed, two unrelated companies scored 0.76 to 0.91 against each other
// on their boilerplate alone, well past the 0.72 review threshold.
//
// A PHRASE, NEVER A WORD. The folded syllables of a legal form are also real
// brand syllables: "cổ", "cô" and "có" all fold to "co", "phần" and "phân" to
// "phan", and "Cỏ May" is a rice company whose whole name folds to "co may". So
// a word of a legal form is only removed as part of the WHOLE phrase, at the
// front of the name. Deleting the token "phan" wherever it appeared would take
// the brand off "Phan Minh".
//
// AND A SHORT LATIN FORM NEEDS A SECOND OPINION. "PT Solutions Physical
// Therapy", "AO World", "AS Roma", "CV Sciences", "AB InBev", "SIA Engineering"
// and "MB Financial" are all real companies whose first word is another market's
// legal form. Position alone cannot tell those from "PT Astra International", so
// an ambiguous marker is believed only when something else about the name agrees
// — see corroboratedMarket.
//
// WHAT THIS IS NOT. Not a second normalizer for the org-name KEY:
// NormalizeOrgName (namesim.go) is unchanged and still produces the exact and
// grouping keys, so no stored key moves. This one is consulted only where two
// names are COMPARED — the gate (orgnamegate.go) and the score (dedupeorg.go).
// The difference matters: "Công ty CP Đầu tư ABC" and "Công ty CP Thương mại
// ABC" must compare as one candidate pair and must NOT group under one key.

import (
	"strings"
	"unicode"
)

// foldDStroke maps đ to d.
//
// U+0111 LATIN SMALL LETTER D WITH STROKE has no canonical decomposition, so
// the NFD pass in normalizeName cannot separate a combining mark from it and
// the letter survives the fold intact: "ĐẦU TƯ" becomes "đau tu", not "dau tu".
// Postgres f_unaccent DOES map it, so without this the same name folds two ways
// — "dau tu" in the database and "đau tu" in Go — and no entry in the tables
// could ever match a name typed with the letter Vietnamese uses for one of its
// most common trade words.
//
// Deliberately NOT inside normalizeName, which is pinned as the input to the
// similarity metric (PO-PARAM-JW-2) and shared with person and lead matching.
// This is an org-name concern and stays on the org-name path.
func foldDStroke(s string) string {
	if !strings.ContainsAny(s, "đĐ") {
		return s
	}
	return strings.NewReplacer("đ", "d", "Đ", "D").Replace(s)
}

// stripLeadingMarkers removes legal forms from the FRONT of a name, longest
// first, for as long as one matches.
//
// Repeated, because the forms stack: "TỔNG CÔNG TY CỔ PHẦN …" carries two, and a
// name may open with a form followed by several trade words.
//
// Longest-match, because the short forms are prefixes of the long ones. Taking
// "cong ty" off "cong ty co phan x" would leave "co phan x" and put the remains
// of a legal form into the comparison.
//
// An AMBIGUOUS marker is skipped unless the caller has already found
// corroboration that this really is the market it looks like.
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

// nameWordSeparators split a name into words, for the legal-form tables here and
// for the gate that compares what is left.
//
// PUNCTUATION SEPARATES. Company names arrive punctuated every way a writer can
// punctuate them — "ACME-Group", "Hewlett.Packard", "Capital.com", "S.C.",
// "Sp. z o.o.", "ООО «Ромашка»", an en dash where a hyphen was meant — and a
// fused token loses a real duplicate: "Acme Ltd" and "ACME-Group Ltd" share the
// word "acme", but as one token "acme-group" it matches nothing.
//
// A CLASS rather than a list, deliberately. An earlier version named six ASCII
// characters and missed the period and the en dash, which is the shape of bug
// that keeps being rediscovered one punctuation mark at a time.
//
// A COMBINING MARK DOES NOT SEPARATE, which the letters-and-digits rule got
// wrong. Thai, Lao, Khmer and the Indic scripts write their vowels as marks
// rather than letters, so that rule cut every Thai word into pieces:
// "เมืองไทย" became four fragments. Those scripts have no spaces between words
// either, so a Thai name is correctly ONE word here.
//
// Deliberately NOT done inside NormalizeOrgName, which also produces exact
// grouping keys (orgMatchKeys in linkedinimport.go, the promotion sweep's
// buckets). Splitting there would make two DIFFERENT names equal as keys.
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
// really belongs to that marker's market.
//
// The question exists because a two-letter Latin form is also an ordinary name.
// "PT Astra International Tbk" is an Indonesian company; "PT Solutions Physical
// Therapy" is an American one, and nothing about the position of "PT"
// distinguishes them.
//
// TWO THINGS COUNT AS AGREEMENT, and both are properties of the name itself
// rather than of the record around it:
//
//   - the market's own bracketing form closes the name. Indonesia's listed
//     companies end in "Tbk", and no English name does.
//   - the rest of the name is not written in Latin script. "ООО Ромашка" and
//     "شركة الراجحي" are unambiguous by their letters, and a Latin marker in
//     front of a Cyrillic or Arabic name is the same evidence.
//
// When neither holds, the marker stays in the name. That keeps today's false
// positive for a genuine "PT Something" pair, which is the safer error: a name
// wrongly stripped matches every company in its sector, while a name left alone
// matches only the ones it already did.
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

// orgNameForMatching is the name reduced to what could identify a company: the
// key with a leading legal form and its trailing trade vocabulary removed.
//
// IT MAY RETURN EMPTY, and that is an answer rather than a failure. A name that
// is nothing but a legal form and trade words — "CÔNG TY CỔ PHẦN THƯƠNG MẠI DỊCH
// VỤ" — has said nothing about WHICH company it is, and two such names have said
// nothing about being the same one. Callers treat empty as no evidence, never as
// a wildcard.
//
// This is the one place it differs from NormalizeOrgName, which must never empty
// a name because it produces a stored key and an empty key would collide with
// every other empty key. A comparison has no such obligation: it is allowed to
// say "I cannot tell these apart on their names".
func orgNameForMatching(s string) string {
	name, _ := matchingFormOf(s)
	return name
}

// matchingFormOf is orgNameForMatching plus the market the name declared, which
// the gate needs: a market whose brands are built from a small pool of shared
// syllables cannot treat one shared word as evidence of identity.
//
// THE TRADE VOCABULARY IS ONLY STRIPPED FROM A NAME THAT DECLARED ITS MARKET.
// Vietnam's abbreviations are two letters — "tm", "dv", "dt", "va" — and two
// letters mean something else everywhere else: "VA Trading" and "DT Robotics"
// are companies whose first word this table would otherwise eat. A legal form is
// a strong enough signal to act on; a bare two-letter word is not.
func matchingFormOf(s string) (string, *marketForms) {
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
