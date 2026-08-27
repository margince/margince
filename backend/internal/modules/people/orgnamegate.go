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
// English and German, the markets whose names this list was measured against.
// A NEW market needs its own generics added here — the list is a precision
// policy, not a language model, and a market whose generics are missing simply
// keeps today's false positives rather than gaining new ones.
//
// A market only belongs here if its generic words are generic AS WORDS. Where a
// name is built by stacking a legal form and trade vocabulary in front of the
// brand, the phrase is what is generic and the words are not, and it is removed
// in position instead — see orgnameforms.go for the Vietnamese case.
var orgNameStopwords = map[string]bool{
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
	// NO VIETNAMESE WORDS HERE, deliberately. A Vietnamese name carries its
	// legal form and trade vocabulary as a multi-word PHRASE in front of the
	// brand, and orgnameforms.go removes it in that position. This map deletes a
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

// orgNameArticles are the words that can never be the whole of a name.
//
// Every other stopword can: "Capital", "Health" and "Digital" are all real
// businesses, so when a strip would empty a name the first word is restored and
// the comparison proceeds. An article is different — "The Group" and "The
// Holding" would both restore to "the" and meet on it, which is two companies
// matching on an English article.
//
// They are stopwords too, folded into orgNameStopwords by init below rather
// than written out twice. One list, one place to add a language.
var orgNameArticles = map[string]bool{
	"the": true, "a": true, "an": true, "and": true, "of": true, "for": true,
	"le": true, "la": true, "les": true, "der": true, "die": true, "das": true,
}

func init() {
	for article := range orgNameArticles {
		orgNameStopwords[article] = true
	}
}

// orgFuzzyTokenFloor is the shortest token a NEAR-identical (rather than equal)
// match is trusted on.
//
// A 0.90 Jaro-Winkler score means something different at each length. On two
// seven-letter tokens it is a spelling variant; on two three-letter tokens it is
// one letter in three, which is how "SORA" met "Suraksha" in the corpus.
// Equality is still honoured at every length — this floor bounds only the fuzzy
// comparison, so "3M" and "HP" match themselves exactly while never matching a
// neighbour.
//
// THREE, and the length was measured rather than guessed. German transliterates
// an umlaut into a second vowel, which makes a short surname pair one rune
// apart: "Bär" folds to "bar" (3) and "Baer" to "baer" (4); "Röhm" to "rohm"
// and "Roehm" to "roehm"; likewise Götz/Goetz and Köhlm/Koehlm. A floor of five
// refused every one of them, and those names are on companies.
//
// Three is safe because the SIMILARITY bar is what separates these, not the
// length. Measured on every three-letter pair that must stay apart —
// hps/kps 0.78, acy/ace 0.82, ibm/ibn 0.82, sora/sura 0.85 — against the
// transliterations that must meet: bar/baer 0.93, rohm/roehm 0.95. The gap is
// clean, and 0.90 sits inside it.
//
// Two is where it stops: a two-letter pair one letter apart has nothing left to
// distinguish it, and equality already covers "3M" and "HP" matching themselves.
const orgFuzzyTokenFloor = 3

// orgGateTokenBudget bounds how many words this gate will compare on one side.
//
// The comparison is a nested loop, so the work is the PRODUCT of the two token
// counts — and `display_name` is `text` with no maxLength in the contract, so
// one create can hand it a megabyte. That would be a slow function anywhere
// else; here it runs inside `DedupeOrganizationForCreate`, which holds the
// workspace-wide organization-name write lock, so an unbounded name would pin
// every organization-name writer in the workspace behind it.
//
// The same reasoning as nameScoringMaxRunes (namesim.go), which bounds the
// length of one comparison. That bound does not help here: it caps each call
// and this is about the NUMBER of calls.
//
// 32 words is far past any real company name — the longest in the measured
// corpus was 6. Past the bound the gate compares the first 32 words of each
// side, which for names this long is the same answer any comparison would give.
const orgGateTokenBudget = 32

// orgFuzzyTokenSimilarity is the bar a token PAIR must clear to count as the
// same word.
//
// Set above the pair scores this is meant to reject and below the
// transliterations it must keep: "Müller"/"Mueller" scores 0.93 and
// "Straße"/"Strasse" 1.00 after the fold, both of which the DACH market
// produces daily.
const orgFuzzyTokenSimilarity = 0.90

// orgFuzzyTokenLengthSlack is how much longer one word may be than the other
// and still be called the same word.
//
// ONE RUNE, which is what a transliteration costs: "bar" becomes "baer", "rohm"
// becomes "roehm", "gotz" becomes "goetz". Every one adds a single vowel.
//
// Without it, Jaro-Winkler's prefix boost makes a short word match any longer
// word that starts with it — "base" against "baseplan" scores exactly 0.90, and
// Base.com and Baseplan are two companies in one real workspace. A shared
// prefix is not a spelling variant, and the similarity bar alone cannot tell
// them apart.
const orgFuzzyTokenLengthSlack = 1

// distinctiveOrgTokens are a name's words with the market's shared vocabulary
// removed.
//
// LENGTH IS NOT A FILTER HERE, and an earlier version that dropped tokens under
// three runes was wrong: it threw away "3M", whose two characters ARE the
// company. A word is dropped for being generic, never for being short.
func distinctiveOrgTokens(name string) []string {
	fields := strings.FieldsFunc(orgNameForMatching(name), nameWordSeparators)
	out := make([]string, 0, min(len(fields), orgGateTokenBudget))
	for i, token := range fields {
		// AN ARTICLE THAT WOULD LEAVE A ONE-WORD NAME IS NOT AN ARTICLE. English
		// puts a real one beside other real words — "Bank of the West" keeps
		// "bank" and "west" without it — so dropping it there costs nothing.
		// A two-word name whose second word is "an" is a different thing: "Việt
		// An", "Long An" and "Hòa An" are companies, and dropping the syllable
		// left each of them a single word that matched half the market.
		if i > 0 && orgNameArticles[token] && len(fields) == 2 {
			out = append(out, token)
			continue
		}
		if orgNameStopwords[token] {
			continue
		}
		if len(out) == orgGateTokenBudget {
			break
		}
		out = append(out, token)
	}
	// THE STRIP MUST NEVER REMOVE THE WHOLE NAME, the same rule
	// NormalizeOrgName follows when a legal suffix IS the company. A name of
	// nothing but generic words still has to be comparable to itself:
	// "Capital" is a real business, "Capital.com" is how it writes itself, and
	// a gate that empties both has nothing left to match them on.
	//
	// The FIRST non-article word, not all of them. Keeping every word would
	// readmit what this gate exists to remove — "Health Care" and "Medical
	// Care" would meet on the shared "care", which is the market's vocabulary
	// and not an identity. Keeping the head of the name is where a brand sits:
	// "Capital" in "Capital.com", "Health" in "Health Care".
	//
	// An article is skipped rather than restored, because it can never be a
	// name: restoring it made "The Group" and "The Holding" meet on "the".
	if len(out) == 0 {
		for _, token := range fields {
			if !orgNameArticles[token] {
				return []string{token}
			}
		}
	}
	return out
}

// sameOrgToken answers whether two words are the same word.
//
// Equal at any length, or near-identical when both are long enough for
// near-identity to mean a spelling variant rather than a different word.
//
// A WORD IS NOT A PREFIX OF ANOTHER WORD. Jaro-Winkler boosts a shared start,
// so a short word scores high against any longer word beginning with it —
// "base" and "baseplan" reach exactly 0.90, and they are two companies. A
// spelling variant changes the letters inside a word, not its length: "rohm"
// and "roehm" differ by one rune, "bar" and "baer" by one. So the two words
// must be within one rune of each other in length before their similarity is
// consulted at all.
func sameOrgToken(a, b string) bool {
	if a == b {
		return true
	}
	lenA, lenB := len([]rune(a)), len([]rune(b))
	if lenA < orgFuzzyTokenFloor || lenB < orgFuzzyTokenFloor {
		return false
	}
	if max(lenA, lenB)-min(lenA, lenB) > orgFuzzyTokenLengthSlack {
		return false
	}
	return jaroWinkler(a, b) >= orgFuzzyTokenSimilarity
}

// squashedOrgName is the name with its word separators removed.
//
// Two things need this, and both are the same difference: where the writer put
// the space. "Health Care" and "Healthcare" are one company; so are "Digital
// Ocean" and "DigitalOcean", "Media Markt" and "MediaMarkt". Compounding is how
// brands are written half the time and it must not decide identity.
//
// It merges exactly that difference and nothing else. "Health Care" and
// "Medical Care" stay apart, because removing spaces does not make different
// words equal.
//
// The same separators the token split uses, so "E-Commerce" and "E Commerce"
// take one path rather than two.
func squashedOrgName(name string) string {
	return strings.Join(strings.FieldsFunc(orgNameForMatching(name), nameWordSeparators), "")
}

// sharesADistinctiveWord is the gate itself: may these two names be scored?
//
// Two ways to answer yes, and the second is not a fallback for the first.
//
// THE SQUASHED NAME is asked first, because compounding hides a shared word
// rather than removing it: "DigitalOcean" is one token, so it shares no word
// with "Digital Ocean" even though they are plainly one company. The same
// comparison is also the whole answer for a name of nothing but market
// vocabulary ("Health Care", "The Group"), which has no distinctive word for
// the token pass to find.
//
// THE TOKEN PASS then asks whether the two names share a word that means
// something — one that is not the vocabulary every company in the market uses.
func sharesADistinctiveWord(a, b string) bool {
	// Where the writer put the space is not evidence of a different company.
	//
	// An EMPTY squashed name is not a match with another empty one: two names
	// made entirely of punctuation have said nothing about being one company,
	// the same way two blank legal names do in bestOrgNamePairing.
	if squashed := squashedOrgName(a); squashed != "" && squashed == squashedOrgName(b) {
		return true
	}
	left, right := distinctiveOrgTokens(a), distinctiveOrgTokens(b)
	_, leftMarket := matchingFormOf(a)
	_, rightMarket := matchingFormOf(b)
	// A name that reduced to NOTHING shares no word and stops here. Vietnamese
	// names carry their legal form and trade vocabulary in front of the brand,
	// so a name made only of those strips to empty (orgnameforms.go) — and two
	// such names have said nothing about being one company, any more than two
	// blank legal names have.
	shared := sharedTokenCount(left, right)
	if shared == 0 {
		return false
	}
	// ONE SHARED WORD IS NOT ALWAYS ENOUGH, and how much it is worth depends on
	// how much of the name it is.
	//
	// In English a distinctive word is nearly the whole of the evidence:
	// "Arvato" appears in "Arvato Systems" and the two are one company. In
	// Vietnamese it is not, because a brand is built from two or three syllables
	// drawn from a small common pool. Measured on the corpus in
	// orgnameforms_test.go: across 30 distinct brands, "nam" and "viet" each
	// appear in 6 of them and "hoa" and "minh" in 3. So "Hòa Bình" and "Hòa
	// Phát" share the word "hoa" — and they are Vietnam's largest construction
	// firm and its largest steelmaker.
	//
	// ASKED ONLY OF A SYLLABIC MARKET, because only there is one shared word weak
	// evidence. Vietnamese builds a brand from two or three syllables drawn from
	// a small common pool, so "Hòa Bình" and "Hòa Phát" share "hoa" and are two
	// different companies. English builds a brand from words that are themselves
	// rare, so one is enough there, and demanding more loses real duplicates:
	// "Amazon AWS" and "Amazon Web Services" share only "amazon" of two words.
	//
	// EITHER side declaring the market is enough. A company does not stop being
	// Vietnamese when someone types its bare brand — "Tân Hiệp Phát" carries no
	// legal form, and it still must not meet "Hòa Phát" on the shared syllable.
	//
	// Strictly more than half, not at least: at half the two names disagree
	// about as much as they agree, and the disagreement is the part that names
	// the company.
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
	// One word of the longer name answers for at most one word of the shorter.
	// Without that, "Acme Acme" drew two matches from the single "acme" in
	// "Acme Beta" and counted a repetition as a second piece of evidence.
	// A name of nothing but common words is still a name, and must find itself.
	// "Bank of Ireland" and "Bank of Ireland Group" are ordinary words end to
	// end, so discounting all of them would leave the company matching nothing.
	//
	// The escape asks that the shorter name appear whole in the longer AND that
	// what the longer adds be a word that names something. "Sun" and "Sun
	// Microsystems" pass: the addition is a brand, so this is one company said
	// twice. "Bank" and "Bank of the West" do not: the addition is "west", and
	// two names built entirely from words every company shares have not said
	// they are one company — a company called Bank must not meet every bank in
	// the estate.
	if agreeEntirely(shorter, longer) {
		return len(shorter)
	}
	taken := make([]bool, len(longer))
	shared := 0
	for _, x := range shorter {
		// A word that many companies share is not evidence that two of them are
		// one company. Two kinds qualify: another market's legal form ("SIA Rimi
		// Latvia" and "SIA Maxima Latvija" are two Latvian grocers, "AS Roma"
		// and "AS Monaco" two football clubs), and the ordinary words that
		// recur across unrelated names in a market ("Bank of the West" against
		// "Bank of the East", "Union Pacific" against "Union Carbide").
		//
		// Discounted HERE rather than removed from the name, so the word still
		// reaches the score. The case where such a word IS the evidence is
		// answered above, before this loop runs.
		if weakEvidenceWord(x) {
			continue
		}
		for j, y := range longer {
			if !taken[j] && sameOrgToken(x, y) {
				taken[j] = true
				shared++
				break
			}
		}
	}
	return shared
}
