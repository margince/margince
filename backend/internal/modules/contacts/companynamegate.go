// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The precondition on the PO-F-2 fuzzy tier: two company names may only be
// SCORED against each other when they share a distinctive word.
//
// `nameSimilarity` is Jaro-Winkler, a character metric with no concept of a
// word. That is right for contact names and wrong for company names, where every
// name in a market ends in the same handful of nouns: measured over one
// workspace's 296 names, 180 pairs reached the review threshold and one was a
// real duplicate. A genuine duplicate shares a whole word — "Arvato Systems"
// contains "arvato" — while the noise shares only letters.
//
// This gate never touches `nameSimilarity`, which is pinned by PO-PARAM-JW-1 and
// shared with contact and lead dedupe, where the character metric is right.

import "strings"

// companyNameStopwords are the words a company name shares with its whole market:
// present in many names, evidence of identity in none. They are REMOVED from a
// name before comparison, so a word that could be a brand belongs in
// companyCommonNameWords instead, which only discounts.
//
// Hand-written and versioned rather than derived from the workspace at run time:
// a derived list would make the same two names match in one workspace and not in
// another, and would put a query inside the transaction holding the
// company-name write lock.
//
// English and German, the markets measured. A market that stacks a legal form
// and trade vocabulary in FRONT of the brand belongs in companyformtables.go
// instead — there the phrase is generic and its words are not.
var companyNameStopwords = map[string]bool{
	// Corporate form that survives NormalizeCompanyName's legal-suffix strip.
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
	// NO VIETNAMESE WORDS HERE, deliberately. A Vietnamese name carries its
	// legal form and trade vocabulary as a multi-word PHRASE in front of the
	// brand, and companynameforms.go removes it in that position. This map deletes a
	// token wherever it appears, which for Vietnamese takes the company with it:
	// "phát" is the second syllable of Hòa Phát, the country's largest
	// steelmaker, and "cổ" folds onto the "cỏ" of the rice company Cỏ May. The
	// syllables are shared across brands, not generic within them.
	// A brand written as its domain — "Capital.com", "Digital.ai" — splits into
	// the name and the top-level domain. The TLD is the most generic token
	// there is: every company that writes its name this way shares one. Without
	// these, "Capital.com" reduces to the single token "com" and matches every
	// other .com in the estate.
	"com": true, "net": true, "org": true, "io": true, "ai": true,
	"app": true, "dev": true, "inc": true, "gmbh": true,
	// Country codes and the second level beneath them. "giba.or.kr" and
	// "utp.or.kr" are two Korean companies, and without "or" and "kr" here they
	// met on the shared domain suffix — the same failure as two ".com"s.
	"kr": true, "jp": true, "cn": true, "vn": true, "uk": true, "de": true,
	"au": true, "nz": true, "sg": true, "in": true, "br": true, "eu": true,
	"or": true, "ne": true, "ac": true, "gov": true, "edu": true,
}

// companyNameArticles are the words that can never be the whole of a name.
//
// Every other stopword can — "Capital" and "Health" are real businesses — so an
// emptied name restores its first word. An article must not be restored: "The
// Group" and "The Holding" would both come back as "the".
//
// Stopwords too, folded in by init below rather than written out twice.
var companyNameArticles = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "of": true, "for": true,
	"le": true, "la": true, "les": true, "der": true, "die": true, "das": true,
}

func init() {
	for article := range companyNameArticles {
		companyNameStopwords[article] = true
	}
}

// companyFuzzyTokenFloor is the shortest token a NEAR-identical (rather than equal)
// match is trusted on. A 0.90 score on two three-letter tokens is one letter in
// three, which is how "SORA" met "Suraksha"; on seven letters it is a spelling
// variant. Equality is honoured at every length, so "3M" still matches itself.
//
// Three rather than five because German transliterates an umlaut into a second
// vowel, and "Bär"/"Baer" and "Röhm"/"Roehm" are company names.
const companyFuzzyTokenFloor = 3

// companyGateTokenBudget bounds how many words this gate compares on one side.
//
// The comparison is a nested loop, so the work is the PRODUCT of the two counts,
// and `display_name` is `text` with no maxLength in the contract. It runs inside
// DedupeCompanyForCreate, which holds the workspace-wide company-name
// write lock, so an unbounded name pins every other writer behind it. The same
// reasoning as nameScoringMaxRunes (namesim.go), which bounds one comparison
// rather than their number.
const companyGateTokenBudget = 32

// companyFuzzyTokenSimilarity is the bar a token PAIR clears to count as the same
// word. It sits in a measured gap: the pairs that must stay apart score 0.78 to
// 0.85, the transliterations the DACH market produces daily score 0.93 and up.
const companyFuzzyTokenSimilarity = 0.90

// companyFuzzyTokenLengthSlack is how much longer one word may be than the other and
// still be the same word. ONE RUNE, which is what a transliteration costs.
//
// Without it Jaro-Winkler's prefix boost makes a short word match any longer one
// starting with it: "base" and "baseplan" score exactly 0.90, and they are two
// companies. A shared prefix is not a spelling variant.
const companyFuzzyTokenLengthSlack = 1

// distinctiveCompanyTokens are a name's words with the market's shared vocabulary
// removed. A word is dropped for being generic, never for being short: a filter
// on length threw away "3M", whose two characters ARE the company.
func distinctiveCompanyTokens(name string) []string {
	fields := strings.FieldsFunc(companyNameForMatching(name), nameWordSeparators)
	out := make([]string, 0, min(len(fields), companyGateTokenBudget))
	for i, token := range fields {
		// An article that would leave a ONE-WORD name is not an article: "Việt
		// An" and "Long An" are companies whose second syllable is the English
		// word, and dropping it left each matching half the market.
		if i > 0 && companyNameArticles[token] && len(fields) == 2 {
			out = append(out, token)
			continue
		}
		if companyNameStopwords[token] {
			continue
		}
		if len(out) == companyGateTokenBudget {
			break
		}
		out = append(out, token)
	}
	// The strip must never remove the whole name: "Capital" is a real business
	// and a gate that empties it has nothing left to match it on.
	//
	// The FIRST non-article word, not all of them — keeping every word would
	// readmit "Health Care" against "Medical Care" on the shared "care".
	if len(out) == 0 {
		for _, token := range fields {
			if !companyNameArticles[token] {
				return []string{token}
			}
		}
	}
	return out
}

// sameCompanyToken answers whether two words are the same word: equal at any length,
// or near-identical when both are long enough for that to mean a spelling
// variant.
//
// A WORD IS NOT A PREFIX OF ANOTHER WORD. Jaro-Winkler boosts a shared start, so
// the length test comes first — a spelling variant changes the letters inside a
// word, not its length.
func sameCompanyToken(a, b string) bool {
	if a == b {
		return true
	}
	lenA, lenB := len([]rune(a)), len([]rune(b))
	if lenA < companyFuzzyTokenFloor || lenB < companyFuzzyTokenFloor {
		return false
	}
	if max(lenA, lenB)-min(lenA, lenB) > companyFuzzyTokenLengthSlack {
		return false
	}
	return jaroWinkler(a, b) >= companyFuzzyTokenSimilarity
}

// squashedCompanyName is the name with its word separators removed, so that where a
// writer put the space does not decide identity: "Health Care" and "Healthcare"
// are one company, and so are "Digital Ocean" and "DigitalOcean".
//
// It merges that difference and nothing else — "Health Care" and "Medical Care"
// stay apart, because removing spaces does not make different words equal.
func squashedCompanyName(name string) string {
	return strings.Join(strings.FieldsFunc(companyNameForMatching(name), nameWordSeparators), "")
}

// sharesADistinctiveWord is the gate itself: may these two names be scored?
//
// The SQUASHED NAME is asked first, because compounding hides a shared word
// rather than removing it — "DigitalOcean" is one token and shares no word with
// "Digital Ocean". The TOKEN PASS then asks whether they share a word that means
// something.
func sharesADistinctiveWord(a, b string) bool {
	// An EMPTY squashed name is not a match with another empty one: two names
	// made entirely of punctuation have said nothing about being one company.
	if squashed := squashedCompanyName(a); squashed != "" && squashed == squashedCompanyName(b) {
		return true
	}
	left, right := distinctiveCompanyTokens(a), distinctiveCompanyTokens(b)
	_, leftMarket := matchingFormOf(a)
	_, rightMarket := matchingFormOf(b)
	// A name that reduced to NOTHING shares no word and stops here: a Vietnamese
	// name of only its legal form and trade vocabulary strips to empty
	// (companynameforms.go), and two such names have said nothing.
	shared := sharedTokenCount(left, right)
	if shared == 0 {
		return false
	}
	// ONE SHARED WORD IS NOT ALWAYS ENOUGH, and this is asked only of a syllabic
	// market. Vietnamese builds a brand from two or three syllables drawn from a
	// small pool — measured over 30 brands, "nam" and "viet" appear in 6 each —
	// so "Hòa Bình" and "Hòa Phát" share "hoa" and are two different companies.
	// English brands are built from rare words, where one is nearly the whole of
	// the evidence and demanding more loses "Amazon AWS" against "Amazon Web
	// Services".
	//
	// EITHER side declaring the market is enough: a company does not stop being
	// Vietnamese when someone types its bare brand.
	if isSyllabicMarket(leftMarket) || isSyllabicMarket(rightMarket) {
		return 2*shared > min(len(left), len(right))
	}
	return true
}

// isSyllabicMarket answers whether a name's market builds its brands from a
// small pool of shared syllables, where one word in common is a coincidence.
func isSyllabicMarket(market *marketForms) bool {
	return market != nil && market.syllabic
}

// sharedTokenCount is how many words of the shorter name appear in the longer.
//
// Counted against the SHORTER side so the answer does not change with argument
// order, and each of its words is counted at most once: a name that repeats a
// word must not accumulate evidence from the repetition.
func sharedTokenCount(left, right []string) int {
	shorter, longer := left, right
	if len(longer) < len(shorter) {
		shorter, longer = longer, shorter
	}
	// A name of nothing but common words must still find itself: "Bank of
	// Ireland" is ordinary words end to end, and discounting all of them would
	// leave the company matching nothing. See agreeEntirely for what it takes.
	if agreeEntirely(shorter, longer) {
		return len(shorter)
	}
	// One word of the longer name answers for at most one word of the shorter,
	// so a name that repeats a word does not accumulate evidence from it.
	taken := make([]bool, len(longer))
	shared := 0
	for _, x := range shorter {
		// Discounted rather than removed, so the word still reaches the score.
		if weakEvidenceWord(x) {
			continue
		}
		for j, y := range longer {
			if !taken[j] && sameCompanyToken(x, y) {
				taken[j] = true
				shared++
				break
			}
		}
	}
	return shared
}
