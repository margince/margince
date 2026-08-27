// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// The string metric behind the dedupe fuzzy tier (PO-F-1/PO-F-2
// `name_sim`). Pinned to standard Jaro-Winkler with prefix scale p=0.1,
// max prefix 4, and no boost threshold (PO-PARAM-JW-1) over casefolded,
// unaccented input (PO-PARAM-JW-2) — pinned so the spec's worked
// examples stay reproducible against this code.
const (
	jaroWinklerPrefixScale = 0.1
	jaroWinklerMaxPrefix   = 4
)

// legalSuffixes is PO-PARAM-1: the trailing tokens org-name
// normalization strips so "Acme Inc" and "Acme GmbH" both reduce to
// "acme" and meet at the fuzzy tier for a human to judge.
var legalSuffixes = map[string]bool{
	"inc": true, "llc": true, "ltd": true, "gmbh": true, "ag": true,
	"sa": true, "sas": true, "bv": true, "oy": true, "plc": true,
	"co": true, "corp": true, "kg": true, "ug": true,
	// The markets that write the form in front ALSO write one behind, and a name
	// carries whichever the writer used. Indonesia's listed companies end in
	// "Tbk" and bracket the brand between it and a leading "PT"; Romania and
	// Turkey put theirs at the end alone.
	//
	// EVERY ENTRY HERE MUST BE A WORD NO COMPANY IS CALLED, because this map
	// feeds NormalizeOrgName, which is a stored grouping key: a word wrongly
	// listed does not merely inflate a score, it files two unrelated companies
	// under one key. Measured, "zoo" (the Polish "z o.o.") made "San Diego Zoo"
	// and "San Diego" the same key, and "as" took the last word off "Trading
	// As". Both are out; the Polish and Nordic forms are not worth that.
	"srl": true, "sro": true, "tbk": true, "oyj": true, "sti": true,
}

// legalConnectives join the halves of a COMPOUND legal form: "GmbH & Co. KG"
// is one form, and a strip that halted on the ampersand left "basecom gmbh &"
// as the key — a name no account is stored under.
//
// They are NOT suffixes in their own right, and treating them as such was
// wrong: it collapsed "Research and" to "research" and "Miller und" to
// "miller", so two unrelated accounts could meet at the same key. A connective
// is consumed only when a real suffix has already been stripped and another
// one follows it — which is exactly the compound case and nothing else.
var legalConnectives = map[string]bool{"&": true, "und": true, "and": true}

// normalizeName casefolds and unaccents (PO-PARAM-JW-2). Both sides of
// every comparison run through it, so the metric stays internally
// consistent; it is deliberately not required to agree rune-for-rune
// with Postgres f_unaccent, which only narrows the candidate set.
// Casefold is Unicode FULL folding, not ToLower — Straße and STRASSE
// must compare equal (ß folds to ss), and the DACH market meets that
// pair daily. A fresh Caser per call: cases.Caser is stateful and not
// safe for concurrent use.
func normalizeName(s string) string {
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

// dropAccents removes the combining marks that are ACCENTS — the ones a writer
// adds to a letter and a reader can do without — and keeps the ones that are
// letters in their own right.
//
// REMOVING EVERY Mn MARK WAS WRONG outside the alphabets this started with. In
// Thai, Lao, Khmer and the Indic scripts a combining mark is a LETTER: the
// vowels hang above and below the consonant instead of sitting beside it.
// Measured, an unrestricted strip turned "เมืองไทย" into "เมองไทย" and
// "កម្ពុជា" into "កមពជា", so every Thai and Khmer company name lost characters
// before any comparison began — and two different names could fold together.
//
// The base letter decides, not the mark. NFD leaves a mark directly after the
// letter it belongs to, so the base says which kind it is:
//
//   - Latin and Greek write ACCENTS, and those go. Müller/Mueller, Straße, Việt
//     Nam and Οδυσσεύς all depend on it.
//   - Hebrew and Arabic write VOCALIZATION — niqqud and harakat — which is
//     optional and usually absent, so a pointed spelling must fold onto the
//     plain one rather than becoming a second company.
//   - Everything else writes LETTERS, and those stay.
//
// CYRILLIC IS NOT IN THAT LIST, and including it was wrong for the same reason
// as Thai. Its marked letters are letters: "й" is not an accented "и" but the
// character in every other Russian word, and stripping it turned "Мойка" into
// "моика" and the Ukrainian "Київстар" into "киівстар". The one real accent
// pair, "ё" against "е", is spelled out below because writers do treat those two
// as interchangeable.
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
			// Russian writes "ё" and "е" for one letter and a registry may hold
			// either, so the DIAERESIS after this one is dropped. NFD has
			// already split "ё" into "е" plus its mark by the time this runs.
			//
			// That mark and no other: "ӗ" is "е" with a BREVE and is a letter of
			// Chuvash, so a rule of "any mark after е" folded "Ӗнер" and "Енер"
			// into one name.
			diaeresisAfterYe = true
			accented = false
		default:
			accented, diaeresisAfterYe = false, false
		}
		out.WriteRune(r)
	}
	return out.String()
}

// NormalizePersonName is the person-name KEY: normalizeName's fold and
// unaccent, plus a collapse of internal whitespace. Both halves are
// load-bearing and neither implies the other.
//
// The fold is what makes Straße and STRASSE one person — strings.ToLower
// leaves ß alone, and the DACH market meets that pair daily. The collapse is
// what makes a page that reflows "Anna  Muster" across a line break the same
// person as "Anna Muster" — a key that kept the second space mints a second
// record on the next crawl of an unchanged page.
//
// Deliberately not normalizeName itself, which also feeds nameSimilarity:
// that input is pinned by PO-PARAM-JW-2 so the spec's worked examples stay
// reproducible against this code.
func NormalizePersonName(s string) string {
	return strings.Join(strings.Fields(normalizeName(s)), " ")
}

// NormalizeOrgName is normalizeName plus the PO-PARAM-1 legal-suffix
// strip, applied only to the trailing token: "Co" inside "Coca Co" is a
// name, "Co" at the end is a suffix.
//
// The strip never consumes the whole name: "Co" alone stays "co", because a
// company may BE its suffix and an empty key would collide with every other
// empty key.
func NormalizeOrgName(s string) string {
	fields := strings.Fields(normalizeName(strings.ReplaceAll(s, ",", " ")))
	strippedOne := false
	for len(fields) > 1 {
		last := undottedForm(fields[len(fields)-1])
		switch {
		case legalSuffixes[last]:
			strippedOne = true
		case legalConnectives[last] && strippedOne && len(fields) > 2 &&
			legalSuffixes[undottedForm(fields[len(fields)-2])]:
			// A connective BETWEEN two legal forms — the "GmbH & Co. KG"
			// case. Consumed only here, so a company whose name simply ends
			// in "and" keeps it.
		default:
			return strings.Join(fields, " ")
		}
		fields = fields[:len(fields)-1]
	}
	return strings.Join(fields, " ")
}

// undottedForm is a trailing word with the dots a writer put inside it removed,
// so a legal form spelled with them is the same form spelled without.
//
// "S.R.L." and "SRL" are one Romanian form, "A.Ş." and "AŞ" one Turkish form,
// and a registry holds whichever the filer typed. Trimming only the FINAL dot
// left "s.r.l." as a single token that matched no entry, so those names kept
// their legal form and scored against every other company that kept the same
// one.
//
// Only dots, and only for the lookup — the word itself is untouched when it
// turns out not to be a legal form, so a name that really contains a dot keeps
// it.
func undottedForm(word string) string {
	if !strings.Contains(word, ".") {
		return word
	}
	return strings.ReplaceAll(word, ".", "")
}

// nameSimilarity is `name_sim`: Jaro-Winkler over normalized input,
// in [0,1].
func nameSimilarity(a, b string) float64 {
	return jaroWinkler(normalizeName(a), normalizeName(b))
}

// jaroWinkler applies the Winkler prefix boost unconditionally — the
// pinned variant has no boost threshold, so a low-Jaro pair with a
// shared prefix still gains (PO-PARAM-JW-1).
// nameScoringMaxRunes bounds what the similarity metric compares.
//
// jaro is quadratic in the longer input — measured on this code, two 60 000-rune
// names take a full second and the cost grows with the square — while
// `display_name` is `text` with no maxLength in the contract, so one create can
// hand it a megabyte. That scoring runs inside the writing transaction holding
// the organization-name lock, so an unbounded score pins a pool connection and
// every organization-name writer in the workspace behind it.
//
// The cap lives HERE and not in normalizeName, which looked like the tidier
// place and is not: normalizeName also produces exact-match and grouping keys
// (orgMatchKeys in linkedinimport.go, the promotion sweep's name buckets), and
// truncating there would make two distinct names compare EQUAL as keys past the
// bound — a match, not a capped score. Capping the metric changes only how
// similar two things are said to be, which is all this bound is entitled to do.
//
// 256 runes is far past any real company or person name. A maxLength in the
// contract is the complete answer and is raised upstream.
const nameScoringMaxRunes = 256

// boundedForScoring caps one side of a comparison. Two names that differ only
// past the bound score as identical, which for names this long is the same
// answer any metric would give.
func boundedForScoring(s string) string {
	if r := []rune(s); len(r) > nameScoringMaxRunes {
		return string(r[:nameScoringMaxRunes])
	}
	return s
}

func jaroWinkler(a, b string) float64 {
	a, b = boundedForScoring(a), boundedForScoring(b)
	j := jaro(a, b)
	if j == 0 {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	prefix := 0
	for prefix < len(ra) && prefix < len(rb) && prefix < jaroWinklerMaxPrefix && ra[prefix] == rb[prefix] {
		prefix++
	}
	return j + float64(prefix)*jaroWinklerPrefixScale*(1-j)
}

// jaro is the standard Jaro similarity: matches inside a half-length
// window, discounted by half the transpositions among them.
func jaro(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}

	window := max(len(ra), len(rb))/2 - 1
	if window < 0 {
		window = 0
	}

	matchedA := make([]bool, len(ra))
	matchedB := make([]bool, len(rb))
	matches := 0
	for i, r := range ra {
		lo := max(0, i-window)
		hi := min(len(rb)-1, i+window)
		for j := lo; j <= hi; j++ {
			if matchedB[j] || rb[j] != r {
				continue
			}
			matchedA[i], matchedB[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}

	// Transpositions: matched runes that pair up out of order. Each such
	// pair is counted twice by this walk, hence the halving below.
	transpositions := 0
	k := 0
	for i := range ra {
		if !matchedA[i] {
			continue
		}
		for !matchedB[k] {
			k++
		}
		if ra[i] != rb[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	return (m/float64(len(ra)) + m/float64(len(rb)) + (m-float64(transpositions)/2)/m) / 3
}

// personNameKeySQL is the SQL side of NormalizePersonName. Both sides of the
// equality arm in fuzzyPerson call it, so that comparison cannot fold its two
// halves with different normalizations of the same name.
//
// Not a mirror of the Go key — it deliberately is not one. SQL's lower and Go's
// Unicode full fold are different normalizations, and the arm exists so the lane
// does not rely on either being a superset of the other. What it DOES owe is
// every pair the Go key calls equal, since that is the equality the lane
// decides on, and three properties of the Go key are not free in SQL:
//
//   - the trim, and the collapse of internal whitespace, because a name a
//     crawled page reflowed across a line break is Go-equal to the same name
//     typed by hand;
//   - Greek final sigma, which full folding maps onto σ and lower() leaves
//     alone, so "Οδυσσεύς" and "ΟΔΥΣΣΕΥΣ" are one person in Go and two here.
//
// Held by TestTheNameKeyArmAdmitsEveryGoEqualPair (creatededupe_integration_test.go),
// which runs this expression against the database over the same pairs the
// reachability test carries — the arm's own claim, asked of the arm rather than
// of the query it sits in, where the trigram arm answers first and hides it.
func personNameKeySQL(expr string) string {
	return storekit.SQLf(
		`btrim(regexp_replace(replace(f_fold_apostrophes(lower(%s)), 'ς', 'σ'), '\s+', ' ', 'g'))`, expr)
}
