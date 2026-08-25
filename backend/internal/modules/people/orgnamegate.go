// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The precondition on the PO-F-2 fuzzy tier: two company names may only be
// SCORED against each other when they share a distinctive word.
//
// WHY THIS EXISTS. `nameSimilarity` is Jaro-Winkler, a character metric with no
// concept of a word. On person names that is right — people have no legal forms
// and no industry vocabulary, so shared letters really are evidence. On company
// names it is not: every company in a market ends in the same handful of nouns,
// and a metric that cannot see a word boundary reads that shared vocabulary as
// shared identity.
//
// MEASURED, on the 296 company names in one real workspace. Of 43,660 pairs,
// 180 scored at or above the 0.72 review threshold. Exactly ONE was a genuine
// duplicate ("Arvato" / "Arvato Systems"). The other 179 were pairs like
// Stripe/Strix (0.89), ACY Capital/Soda Capital (0.82), Fenwick
// Software/Centric Software (0.82), HPS/KPS (0.72). A 99.4% false-positive
// rate, firing on every organization create.
//
// It was not theoretical. A CSV import of 107 contacts refused to create nine
// companies, reporting "a company of this name is already in the CRM" when no
// such company existed — the operator went looking through the UI for records
// that were never there.
//
// THE RULE. A genuine duplicate shares a whole distinctive word; the noise
// shares only letters. "Arvato Systems" contains "arvato"; "Strix" contains no
// word of "Stripe". So the score is only consulted once a shared distinctive
// word is found. The same 180 pairs reduce to 3.
//
// WHAT THIS DOES NOT CHANGE. Not `nameSimilarity`, which stays exactly the
// pinned Jaro-Winkler (PO-PARAM-JW-1) and stays shared with person dedupe
// (dedupe.go), lead dedupe (leaddedupe.go) and site-person matching
// (sitepersonfields.go), where the character metric is the right one. Not tier
// 1 (`exactOrgByDomain`), which returns before this tier is reached — a domain
// is an exact key and needs no name evidence at all.

import "strings"

// orgNameStopwords are the words a company name shares with its whole market:
// present in many names, evidence of identity in none.
//
// A HAND-WRITTEN, VERSIONED LIST, and deliberately not one derived from the
// workspace's own names at run time. A derived list would make the same two
// names match in one workspace and not in another, drift as each workspace
// grew, and put a second query inside `DedupeOrganizationForCreate`, which
// holds the organization-name write lock while the ladder runs. Every other
// dedupe parameter in this package is an auditable constant (dedupe.go), and
// this one is read the same way.
//
// English, German and Vietnamese, because those are the markets the estate
// currently holds. A NEW market needs its own generics added here — the list is
// a precision policy, not a language model, and a market whose generics are
// missing simply keeps today's false positives rather than gaining new ones.
//
// "phat" earns its place the same way "group" does: it is Vietnamese for
// "prosper" and ends company names the way "Group" ends English ones. Measured
// in the corpus, it appeared in three unrelated names.
var orgNameStopwords = map[string]bool{
	// Structural.
	"the": true, "and": true, "of": true, "for": true,
	// Corporate form that survives NormalizeOrgName's legal-suffix strip.
	"group": true, "holding": true, "holdings": true, "company": true,
	"international": true, "global": true, "worldwide": true, "national": true,
	// Sector vocabulary.
	"digital": true, "technology": true, "technologies": true, "tech": true,
	"solutions": true, "systems": true, "services": true, "software": true,
	"consulting": true, "consultants": true, "partners": true, "associates": true,
	"capital": true, "ventures": true, "investments": true, "financial": true,
	"health": true, "healthcare": true, "care": true, "medical": true,
	"hospital": true, "clinic": true,
	"engineering": true, "industries": true, "manufacturing": true,
	"media": true, "marketing": true, "communications": true, "logistics": true,
	// Vietnamese generics.
	"cong": true, "ty": true, "co": true, "phat": true,
}

// orgFuzzyTokenFloor is the shortest token a NEAR-identical (rather than equal)
// match is trusted on.
//
// A 0.90 Jaro-Winkler score means something different at each length. On two
// seven-letter tokens it is a spelling variant; on two four-letter tokens it is
// one letter, which is how "SORA" met "Suraksha" in the corpus. Equality is
// still honoured at every length — this floor bounds only the fuzzy comparison,
// so "3M" and "HP" match themselves exactly while never matching a neighbour.
const orgFuzzyTokenFloor = 5

// orgFuzzyTokenSimilarity is the bar a token PAIR must clear to count as the
// same word.
//
// Set above the pair scores this is meant to reject and below the
// transliterations it must keep: "Müller"/"Mueller" scores 0.93 and
// "Straße"/"Strasse" 1.00 after the fold, both of which the DACH market
// produces daily.
const orgFuzzyTokenSimilarity = 0.90

// orgTokenSeparators split a name into words for THIS gate's purposes.
//
// A hyphen or a slash joins two words in one company name — "ACME-Group",
// "Rich-Media/Solutions" — and `strings.Fields` alone leaves them fused. That
// fusion loses real duplicates: "Acme Ltd" and "ACME-Group Ltd" share the word
// "acme", but as one token "acme-group" it matches nothing.
//
// Deliberately NOT done inside NormalizeOrgName, which also produces exact
// grouping keys (orgMatchKeys in linkedinimport.go, the promotion sweep's
// buckets). Splitting there would make two DIFFERENT names equal as keys.
// Splitting here changes only which words this gate compares.
func orgTokenSeparators(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '/' || r == '_'
}

// distinctiveOrgTokens are a name's words with the market's shared vocabulary
// removed.
//
// LENGTH IS NOT A FILTER HERE, and an earlier version that dropped tokens under
// three runes was wrong: it threw away "3M", whose two characters ARE the
// company. A word is dropped for being generic, never for being short.
func distinctiveOrgTokens(name string) []string {
	fields := strings.FieldsFunc(NormalizeOrgName(name), orgTokenSeparators)
	out := make([]string, 0, len(fields))
	for _, token := range fields {
		if !orgNameStopwords[token] {
			out = append(out, token)
		}
	}
	return out
}

// sameOrgToken answers whether two words are the same word.
//
// Equal at any length, or near-identical when both are long enough for
// near-identity to mean a spelling variant rather than a different word.
func sameOrgToken(a, b string) bool {
	if a == b {
		return true
	}
	if len([]rune(a)) < orgFuzzyTokenFloor || len([]rune(b)) < orgFuzzyTokenFloor {
		return false
	}
	return jaroWinkler(a, b) >= orgFuzzyTokenSimilarity
}

// squashedOrgName is the name with its word separators removed, for the one
// case that has no distinctive word to compare.
//
// "Health Care" and "Healthcare" are the same company written two ways, and
// every word in both is generic. Requiring exact equality there would lose the
// pair over a space. Removing the separators merges exactly that difference and
// nothing else: "Health Care" and "Medical Care" stay apart, because squashing
// does not make different words equal.
//
// The same separators the token split uses, so "E-Commerce" and "E Commerce"
// take one path rather than two.
func squashedOrgName(name string) string {
	return strings.Join(strings.FieldsFunc(NormalizeOrgName(name), orgTokenSeparators), "")
}

// sharesADistinctiveWord is the gate itself: may these two names be scored?
//
// The ALL-GENERIC fallback is the interesting branch. A name made entirely of
// market vocabulary ("Health Care", "The Group") has nothing distinctive to
// match on, so the character metric would be back to comparing letters with
// nothing to anchor them. Comparing the squashed names instead answers the only
// question left — is this the same generic name written differently — without
// readmitting the noise.
func sharesADistinctiveWord(a, b string) bool {
	left, right := distinctiveOrgTokens(a), distinctiveOrgTokens(b)
	if len(left) == 0 || len(right) == 0 {
		return squashedOrgName(a) == squashedOrgName(b)
	}
	for _, x := range left {
		for _, y := range right {
			if sameOrgToken(x, y) {
				return true
			}
		}
	}
	return false
}
