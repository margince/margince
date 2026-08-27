// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Which shared words count as evidence that two company names are one company.
//
// The gate next door (orgnamegate.go) decides whether two names may be scored at
// all, and it does that by looking for a word they share. That is right when the
// word is a brand and wrong when it is the vocabulary of a whole market: "Bank of
// the West" and "Bank of the East" share "bank", "Union Pacific" and "Union
// Carbide" share "union", and each pair is two different businesses.

// orgCommonNameWords are ordinary words that many unrelated companies put in
// their names, so two names meeting on one have said nothing about being one
// company.
//
// DIFFERENT FROM orgNameStopwords, which is why this is a second map rather than
// more entries in that one. A stopword is REMOVED from the name: it is market
// vocabulary and never a brand, so "Digital Solutions" and "Digital Ocean" must
// not meet on either word. These words CAN be a brand — Bank of America is a
// bank, Central Group is a real company, Star Alliance is named Star — so they
// stay in the name and reach the score, and only stop being counted as the
// evidence that two names are one company.
//
// Measured on the pairs that reached review wrongly: "Bank of the West" against
// "Bank of the East" scored 0.9750, "Union Pacific" against "Union Carbide"
// 0.8547, "Central Park Media" against "Central Valley Media" 0.8967. Each
// shares exactly one ordinary word and nothing else.
//
// SMALL ON PURPOSE. This is not a frequency list and must not grow into one: a
// long list starts refusing real brands their only evidence. What keeps a
// company actually called "Bank" findable is the escape in sharedTokenCount —
// two names that agree entirely are one company however ordinary their words —
// and a market whose common words are missing keeps today's false positives
// rather than gaining new ones.
//
// The countries are here for the same reason as the rest: a country says where a
// company works, not which company it is, and a subsidiary carries its parent's
// country the way a rival carries the same one.
var orgCommonNameWords = map[string]bool{
	// Sector nouns that name a whole industry rather than a company in it.
	"bank": true, "insurance": true, "energy": true, "motors": true, "airlines": true,
	"telecom": true, "pharma": true, "foods": true, "realty": true, "properties": true,
	// Qualifiers a company adds to say it is large, old or first.
	"first": true, "premier": true, "prime": true, "standard": true, "general": true,
	"united": true, "allied": true, "associated": true, "union": true, "central": true,
	"metro": true, "regional": true, "continental": true, "universal": true,
	// Direction and place words that qualify a name rather than being one.
	"north": true, "south": true, "east": true, "west": true, "northern": true,
	"southern": true, "eastern": true, "western": true, "pacific": true, "atlantic": true,
	"american": true, "european": true, "asia": true, "asian": true,
	// Ornaments.
	"royal": true, "crown": true, "star": true, "sun": true, "summit": true, "apex": true,
	// The country a company works in, which a subsidiary carries and a rival
	// carries too: "Rimi Latvia" and "Maxima Latvija" are two Latvian grocers,
	// and two unrelated firms both add "Deutschland" to their German arm.
	//
	// The MARKETS THIS TREE HOLDS, in the spellings those markets use — the same
	// rule as the rest of this map, and the same rule as the legal-form tables
	// next door. A country is added when a name from it arrives, not in advance:
	// a list of every country would refuse evidence to companies nobody here has.
	"latvia": true, "latvija": true, "deutschland": true, "france": true,
	"ireland": true, "indonesia": true,
}

// weakEvidenceWord answers whether a shared word says nothing about two names
// being one company.
func weakEvidenceWord(word string) bool {
	return ambiguousFormWord[word] || orgCommonNameWords[word]
}

// anyStrongWord answers whether a name has a word of its own — one that is not
// vocabulary every company shares.
func anyStrongWord(fields []string) bool {
	for _, word := range fields {
		if !weakEvidenceWord(word) {
			return true
		}
	}
	return false
}

// agreeEntirely answers whether the shorter name appears whole in the longer,
// and whatever the longer adds is a word that names something rather than one
// every company shares.
//
// A MULTISET TEST, not a subsequence one — word order is not consulted, because
// a registry and a letterhead disagree about it more often than two companies
// do. "Union Pacific Bank" and "Pacific Union Bank" therefore agree, which is
// the answer a human would give.
//
// The condition on the LEFTOVER is what separates a qualified brand from two
// unrelated names: "Sun Microsystems" adds a brand to "Sun", while "Bank of the
// West" adds only "west" to "Bank", and a pair whose every word is market
// vocabulary has nothing left to be identified by.
func agreeEntirely(shorter, longer []string) bool {
	if len(shorter) == 0 {
		return false
	}
	// A name of nothing but common words may only agree with a name of exactly
	// the same words. "Bank of Ireland" and "Bank of Ireland Group" are one
	// company — "group" is deleted upstream, so both arrive as the same two
	// words. "First National Bank" arrives as "first bank" and "First Republic
	// Bank" as "first republic bank": one is a strict subset of the other, both
	// are vocabulary end to end, and there is no company between them to find.
	//
	// A one-word name is exempt, because a company called "Sun" has said all it
	// has to say and "Sun Microsystems" is that name qualified.
	//
	// Comparing LENGTHS is enough to mean "a subset rather than the same words":
	// two names of equal length that differ in a word are rejected by the
	// containment loop below, which needs every word of the shorter to be found.
	if len(shorter) > 1 && !anyStrongWord(shorter) && len(shorter) != len(longer) {
		return false
	}
	taken := make([]bool, len(longer))
	for _, x := range shorter {
		found := false
		for j, y := range longer {
			if !taken[j] && sameOrgToken(x, y) {
				taken[j], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	for j, y := range longer {
		if !taken[j] && weakEvidenceWord(y) {
			return false
		}
	}
	return true
}
