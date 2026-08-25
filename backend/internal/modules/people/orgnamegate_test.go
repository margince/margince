// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "testing"

// The pairs that sent an operator looking for records that did not exist.
//
// Each is a real name from one workspace, and each scored at or above the 0.72
// review threshold on Jaro-Winkler alone. None is a duplicate of the other.
// This is the regression: the character metric said yes and the estate said no.
func TestUnrelatedCompaniesAreNotScoredAgainstEachOther(t *testing.T) {
	unrelated := [][2]string{
		// The nine that broke a CSV import of 107 contacts.
		{"Bytesforce", "Salesforce"},
		{"ACY Capital", "Soda Capital"},
		{"Fenwick Software", "Centric Software"},
		{"Pronto Software", "Centric Software"},
		{"TLC Healthcare", "HealthCare Logic"},
		{"The NMG Group", "The Performance Network Group (TPNG)"},
		{"The Alta Group", "Thinksmart Group"},
		{"Multimedia Technology", "MTA TECH"},
		{"Digital Matter", "Elephant Digital"},
		// The highest-scoring noise in the same corpus.
		{"Stripe", "Strix"},
		{"HPS", "KPS"},
		{"FIS", "igus"},
		{"EMS", "OPMS"},
		{"netcare", "Newtrend"},
		{"adesso", "ADSSI"},
		{"SORA", "Suraksha"},
		// Generic words shared, nothing else.
		{"The Group", "The Holding"},
		{"Health Care", "Medical Care"},
	}
	for _, pair := range unrelated {
		got := bestOrgNamePairing(pair[0], "", pair[1], "").Confidence
		if got >= dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, at or above the %.2f review threshold — "+
				"these are different companies",
				pair[0], pair[1], got, dedupeReviewThreshold)
		}
	}
}

// The gate must not cost recall. Every pair here is one company written two
// ways, and every one has to survive.
func TestRealDuplicatesStillMeetAtTheFuzzyTier(t *testing.T) {
	duplicates := [][2]string{
		// The legal-suffix case the fuzzy tier exists for (PO-PARAM-1).
		{"Acme Inc", "Acme GmbH"},
		{"Siemens AG", "Siemens"},
		{"basecom GmbH & Co. KG", "basecom"},
		{"Shopware AG", "shopware"},
		// A qualifier added or dropped.
		{"Arvato", "Arvato Systems"},
		{"Sun", "Sun Microsystems"},
		{"The NMG Group", "NMG Group"},
		{"Fenwick Software", "Fenwick Software Pty Ltd"},
		{"ACY Capital", "ACY Capital Pty Ltd"},
		{"Digital Matter", "Digital Matter Pty Ltd"},
		// Transliteration, which the DACH market produces daily.
		{"Müller Software", "Mueller Software"},
		{"Straße Handel", "Strasse Handel"},
		// Spacing and compounding.
		{"TLC Healthcare", "TLC Health Care"},
		{"Pricewaterhouse Coopers", "PricewaterhouseCoopers"},
		{"Dynamic-QS", "Dynamic QS"},
		{"E-Commerce Ltd", "Ecommerce Ltd"},
		// A hyphen joins two words, and treating the pair as one token lost
		// this: "acme-group" matches nothing, while "acme" matches "acme".
		{"Acme Ltd", "ACME-Group Ltd"},
		{"Rich Media", "Rich-Media Solutions"},
		// A short name IS the company: length is not a filter.
		{"3M", "3M Company"},
		{"Box", "Box Inc"},
		{"IBM", "IBM"},
		{"HPS", "HPS"},
		// Every word generic, so there is no distinctive token to gate on.
		{"Health Care", "Healthcare"},
		{"The Group", "The Group Ltd"},
		{"Digital Solutions", "Digital Solutions GmbH"},
	}
	for _, pair := range duplicates {
		got := bestOrgNamePairing(pair[0], "", pair[1], "").Confidence
		if got < dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f, below the %.2f review threshold — "+
				"this is one company and the queue will never see it",
				pair[0], pair[1], got, dedupeReviewThreshold)
		}
	}
}

// Length is not evidence that a word means nothing.
//
// An earlier version of the gate dropped tokens under three runes as
// insignificant, which threw away "3M" — whose two characters are the entire
// company — and would have turned a working match into a miss.
func TestAShortNameIsStillAName(t *testing.T) {
	for _, name := range []string{"3M", "HP", "GE", "Box", "Sun", "IBM"} {
		if got := distinctiveOrgTokens(name); len(got) == 0 {
			t.Errorf("%q has no distinctive token, so it can never match itself", name)
		}
	}
}

// Near-identity is only trusted where it means a spelling variant.
//
// On two long words a 0.90 score is an umlaut or a doubled letter. On two short
// ones it is a different word: "sora" and "sura" differ by one character out of
// four. Equality is honoured at every length, which is what keeps "3M" working.
func TestNearIdentityIsOnlyTrustedOnLongEnoughWords(t *testing.T) {
	if sameOrgToken("sora", "sura") {
		t.Error("two four-letter words one character apart counted as the same word")
	}
	if !sameOrgToken("3m", "3m") {
		t.Error("a short word no longer matches itself exactly")
	}
	if !sameOrgToken("mueller", "muller") {
		t.Error("the ue/ü transliteration no longer meets, and the DACH market produces it daily")
	}
}

// A name of nothing but market vocabulary has no distinctive word, and the
// fallback answers the only question left: is this the same generic name
// written differently?
func TestAnAllGenericNameFallsBackToTheSquashedName(t *testing.T) {
	if !sharesADistinctiveWord("Health Care", "Healthcare") {
		t.Error("one company written with and without a space no longer meets")
	}
	if sharesADistinctiveWord("Health Care", "Medical Care") {
		t.Error("two different generic names met — squashing must not make different words equal")
	}
}

// The gate is organization-only. `nameSimilarity` stays the pinned Jaro-Winkler
// (PO-PARAM-JW-1) that person dedupe, lead dedupe and site-person matching all
// read, where a character metric is the right one: people carry no legal forms
// and no industry vocabulary.
func TestTheGateDoesNotReachPersonNameSimilarity(t *testing.T) {
	// Two unrelated people whose names merely look alike still score high,
	// because that is the person ladder's job to weigh against other evidence.
	if got := nameSimilarity("Jon Smith", "Jan Smith"); got < 0.8 {
		t.Errorf("person name similarity = %.4f, want the unchanged metric", got)
	}
}

// Classes no name metric reaches, before this change or after.
//
// Written down so a reader does not mistake the gate for their cause. Each pair
// is one company, and each scores below the review threshold on the ORIGINAL
// metric — the gate takes nothing away here. They are answered by tier 1, the
// exact domain match, which needs no name evidence at all.
func TestClassesThatOnlyDomainEvidenceCanAnswer(t *testing.T) {
	unreachable := [][2]string{
		{"IBM", "International Business Machines"},
		{"BMW", "Bayerische Motoren Werke"},
		{"Facebook", "Meta"},
		{"Сбербанк", "Sberbank"},
		{"삼성전자", "Samsung Electronics"},
	}
	for _, pair := range unreachable {
		if got := nameSimilarity(NormalizeOrgName(pair[0]), NormalizeOrgName(pair[1])); got >= dedupeReviewThreshold {
			t.Errorf("%q vs %q scores %.4f on the raw metric — this test's premise "+
				"is that name similarity cannot reach it, and that has changed",
				pair[0], pair[1], got)
		}
	}
}
