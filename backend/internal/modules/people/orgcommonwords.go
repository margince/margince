// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Which shared words count as evidence that two company names are one company.
//
// The gate next door looks for a word two names share. That is right when the
// word is a brand and wrong when it is the vocabulary of a whole market: "Bank
// of the West" and "Bank of the East" share "bank", and they are two banks.

// orgCommonNameWords are ordinary words that many unrelated companies put in
// their names.
//
// DISCOUNTED, NOT DELETED, which is why this is not more entries in
// orgNameStopwords. A stopword is removed from the name because it is never a
// brand; these words CAN be one — Bank of America is a bank, Star Alliance is
// named Star — so they stay in the name and reach the score, and only stop
// counting as the evidence that two names are one company.
//
// SMALL ON PURPOSE, and not a frequency list: a long one starts refusing real
// brands their only evidence. What keeps a company actually called "Bank"
// findable is agreeEntirely below. A market whose common words are missing keeps
// today's false positives rather than gaining new ones.
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
	// carries too: "Rimi Latvia" and "Maxima Latvija" are two Latvian grocers.
	// Added when a name from that market arrives, not in advance.
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
	// the same words. Where one is a strict SUBSET of the other and both are
	// vocabulary end to end, there is no company between them to find.
	//
	// A one-word name is exempt: a company called "Sun" has said all it has to
	// say, and "Sun Microsystems" is that name qualified.
	//
	// Comparing LENGTHS is enough to mean "a subset rather than the same words",
	// because the containment loop below rejects equal-length names that differ.
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
