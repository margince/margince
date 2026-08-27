// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "testing"

// TWO COMPANIES THAT SHARE ONE ORDINARY WORD ARE NOT ONE COMPANY.
//
// A shared word is evidence of identity when it is a brand and not when it is
// the vocabulary of a whole market. Every pair here shares exactly one such word
// and nothing else.
func TestTwoCompaniesSharingOneOrdinaryWordStayApart(t *testing.T) {
	for _, p := range [][2]string{
		{"Bank of the West", "Bank of the East"},
		{"Union Pacific", "Union Carbide"},
		{"Central Park Media", "Central Valley Media"},
		{"Pacific Gas", "Pacific Life"},
		{"Royal Bank", "Royal Mail"},
		{"North Star", "South Star"},
		{"Standard Bank", "Standard Life"},
		{"Premier Foods", "Premier Energy"},
		// A country is the same kind of word: it says where a company works,
		// not which company it is.
		{"SIA Rimi Latvia", "SIA Maxima Latvija"},
		{"Acme Deutschland", "Beta Deutschland"},
		{"Foo France", "Bar France"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got >= dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, at or above the %.2f review threshold "+
				"— they share market vocabulary, not a company",
				p[0], p[1], got, dedupeReviewThreshold)
		}
	}
}

// AND A COMMON WORD IS STILL EVIDENCE WHEN IT IS THE WHOLE NAME.
//
// Discounting a word must not stop a company finding itself. "Bank of Ireland"
// is ordinary words end to end, and "Bank" is a real company name; refusing them
// their only evidence would leave each matching nothing.
//
// The escape is that the shorter name appears WHOLE inside the longer one. "Bank
// of Ireland" and "Bank of Ireland Group" agree on every word they have, while
// "Bank of the West" and "Bank of the East" agree on one and disagree on the
// word that names them.
func TestACommonWordIsEvidenceWhenItIsTheWholeName(t *testing.T) {
	for _, p := range [][2]string{
		{"Bank of Ireland", "Bank of Ireland Group"},
		{"Air France", "Air France KLM"},
		{"Bank", "Bank Ltd"},
		{"Star", "Star GmbH"},
		{"Central", "Central Inc"},
		{"France", "France SA"},
		{"Bank of America", "Bank of America Corp"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got < dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, below the %.2f review threshold — one "+
				"of these names is the whole of the other",
				p[0], p[1], got, dedupeReviewThreshold)
		}
	}
	// AND THE ESCAPE REACHES NO FURTHER. A bare common word must not meet every
	// longer name that begins with it: what the longer name ADDS has to name
	// something. "Sun Microsystems" adds a brand to "Sun" and meets it; "Bank of
	// the West" adds only "west" to "Bank" and does not.
	for _, p := range [][2]string{
		{"Bank", "Bank of the West"},
		{"United", "United Airlines"},
		{"Metro", "Metro Bank"},
		{"Star", "North Star"},
		{"Prime", "Prime Standard"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got >= dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f — a company named after one ordinary "+
				"word must not meet every longer name carrying it",
				p[0], p[1], got)
		}
	}
	// A distinctive addition IS a qualified version of the same brand.
	for _, p := range [][2]string{
		{"Sun", "Sun Microsystems"},
		{"Star", "Star Alliance"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got < dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, below the %.2f threshold — the longer "+
				"name adds a brand, which makes this one company said twice",
				p[0], p[1], got, dedupeReviewThreshold)
		}
	}
	// Finding: order is not consulted, which a reader must not have to guess.
	if got := bestOrgNamePairing("Union Pacific Bank", "", "Pacific Union Bank", "").Confidence; got < dedupeReviewThreshold {
		t.Errorf("two names of the same words in a different order score %.4f — "+
			"a registry and a letterhead disagree about order more often than "+
			"two companies do", got)
	}
}

// A NAME OF NOTHING BUT COMMON WORDS AGREES ONLY WITH THE SAME WORDS.
//
// "First National Bank" reduces to "first bank" — market vocabulary end to end,
// because "national" is deleted upstream as a stopword — and that is a strict
// subset of "first republic bank". The addition reads as distinctive and is not:
// there is no company between the two names, only words half the market uses.
//
// "Bank of Ireland" is the shape this must not catch: it reduces to the SAME two
// words as "Bank of Ireland Group", not to a subset of them.
func TestAnAllCommonNameAgreesOnlyWithTheSameWords(t *testing.T) {
	for _, p := range [][2]string{
		{"First National Bank", "First Republic Bank"},
		{"United Pacific Group", "United Atlantic Group"},
		{"Central National Bank", "Central Regional Bank"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got >= dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f — one name is a subset of the other and "+
				"every word of both is vocabulary, so there is no company between them",
				p[0], p[1], got)
		}
	}
	// Same words, one of them qualified away: still one company.
	for _, p := range [][2]string{
		{"Bank of Ireland", "Bank of Ireland Group"},
		{"Air France", "Air France KLM"},
		{"Union Pacific Bank", "Pacific Union Bank"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got < dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, below the %.2f threshold — these are "+
				"the same words", p[0], p[1], got, dedupeReviewThreshold)
		}
	}
}

// The discount must not reach a name built from rare words, which is where one
// shared word really is nearly the whole of the evidence.
func TestARareSharedWordIsStillEvidence(t *testing.T) {
	for _, p := range [][2]string{
		{"Arvato", "Arvato Systems"},
		{"Amazon AWS", "Amazon Web Services"},
		{"Siemens", "Siemens Energy"},
		{"Bosch", "Bosch Rexroth"},
		{"Deloitte", "Deloitte Consulting"},
		{"Fenwick Software", "Fenwick Software Pty Ltd"},
		{"TLC Healthcare", "TLC Health Care"},
		{"PT Solutions Physical Therapy", "PT Solutions"},
	} {
		if got := bestOrgNamePairing(p[0], "", p[1], "").Confidence; got < dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, below the %.2f review threshold — this "+
				"is one company and the word they share is its name",
				p[0], p[1], got, dedupeReviewThreshold)
		}
	}
}

// Every entry must be written the way a folded name is written, or it can never
// fire and no other test would notice.
func TestCommonNameWordsMatchTheirOwnNormalization(t *testing.T) {
	if len(orgCommonNameWords) == 0 {
		t.Fatal("no common words declared — the census would pass against nothing")
	}
	for word := range orgCommonNameWords {
		if got := NormalizeOrgName(word); got != word {
			t.Errorf("%q normalizes to %q — written this way it can never match a "+
				"word in a real name", word, got)
		}
		// A word here must not ALSO be a stopword: the two lists mean different
		// things, and a word in both is deleted before this one is consulted,
		// which makes its entry here dead.
		if orgNameStopwords[word] {
			t.Errorf("%q is both a stopword and a common word — the stopword "+
				"deletes it first, so the entry here never fires", word)
		}
	}
}
