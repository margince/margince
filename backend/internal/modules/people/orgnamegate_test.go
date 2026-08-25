// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"strings"
	"testing"
)

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
		// The control on the squashed-name comparison: two companies that
		// share a generic FIRST word are not one company, however the
		// separators fall.
		{"Capital One", "Capital Group"},
		{"Digital Ocean", "Digital Matter"},
		{"Media Markt", "Media Monks"},
		{"Health Engine", "Health Direct"},
		// A word is not a prefix of another word. Jaro-Winkler boosts a shared
		// start, so "base" reaches exactly 0.90 against "baseplan" — and these
		// are two companies in one real workspace.
		{"Base.com", "Baseplan"},
		// Two domains share their suffix, not their identity. Without the
		// country code and the level beneath it, both of these reduce to "or".
		{"giba.or.kr", "utp.or.kr"},
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
		// A generic word compounded onto the distinctive one. Half of all
		// brands are written both ways, and the compound form leaves the token
		// pass nothing to match: "digitalocean" shares no word with "digital
		// ocean". These are the pairs the squashed-name comparison exists for.
		{"Digital Ocean", "DigitalOcean"},
		{"Media Markt", "MediaMarkt"},
		{"Tech Data", "TechData Corp"},
		{"Health Engine", "HealthEngine"},
		// A company whose whole name is a word this gate calls generic.
		{"Capital", "Capital Ltd"},
		{"Capital One", "Capital One Financial"},
		// A brand written as its own domain. The strip must not empty the name
		// and leave the top-level domain as its identity.
		{"Capital", "Capital.com"},
		{"Digital", "Digital.ai"},
		{"Media", "Media.net"},
		// Punctuation is a word separator, whatever punctuation it is. An
		// earlier version listed six ASCII characters and missed the period.
		{"Hewlett Packard", "Hewlett.Packard"},
		// Short transliterations. German turns an umlaut into a second vowel,
		// which lands these at three and four runes.
		{"Bär", "Baer"},
		{"Röhm", "Roehm"},
		{"Götz", "Goetz"},
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

// Where the writer put the space is not evidence of a different company.
//
// This is asked BEFORE the token pass, not as a fallback after it: compounding
// hides a shared word rather than removing one. "DigitalOcean" is a single
// token, so it shares no word with "Digital Ocean" — but they are one company,
// and the token pass alone would never say so.
func TestCompoundingDoesNotDecideIdentity(t *testing.T) {
	same := [][2]string{
		{"Health Care", "Healthcare"},
		{"Digital Ocean", "DigitalOcean"},
		{"Media Markt", "MediaMarkt"},
		{"E-Commerce", "E Commerce"},
	}
	for _, pair := range same {
		if !sharesADistinctiveWord(pair[0], pair[1]) {
			t.Errorf("%q and %q no longer meet — they differ only in spacing",
				pair[0], pair[1])
		}
	}
	// Squashing merges the separators and nothing else: it must not make two
	// different words equal.
	different := [][2]string{
		{"Health Care", "Medical Care"},
		{"Digital Ocean", "Digital Matter"},
	}
	for _, pair := range different {
		if sharesADistinctiveWord(pair[0], pair[1]) {
			t.Errorf("%q and %q met — squashing must not make different words equal",
				pair[0], pair[1])
		}
	}
}

// The comparison is a nested loop, so its cost is the PRODUCT of the two token
// counts — and it runs inside DedupeOrganizationForCreate, which holds the
// workspace-wide organization-name write lock. `display_name` is `text` with no
// maxLength in the contract, so one create can hand it a megabyte; unbounded,
// that would pin every organization-name writer in the workspace behind it.
//
// The 256-rune cap in jaroWinkler does not help: it bounds each call, and this
// is about the number of calls.
func TestTheTokenLoopIsBounded(t *testing.T) {
	huge := strings.Repeat("acme ", 20000)
	if got := len(distinctiveOrgTokens(huge)); got > orgGateTokenBudget {
		t.Errorf("a 20,000-word name yields %d tokens, past the %d budget — the "+
			"comparison is quadratic and runs under the name write lock",
			got, orgGateTokenBudget)
	}
	// And the bound must not cost a real name: the longest in the measured
	// corpus was six words.
	if got := len(distinctiveOrgTokens("Dynamic Air Quality Solutions Pty Ltd")); got == 0 {
		t.Error("an ordinary six-word name lost all its tokens")
	}
}

// A word is not a prefix of another word.
//
// Jaro-Winkler boosts a shared start, so a short word scores high against any
// longer word beginning with it. A spelling variant changes the letters inside
// a word, not its length.
func TestAPrefixIsNotASpellingVariant(t *testing.T) {
	if sameOrgToken("base", "baseplan") {
		t.Error("a word matched a longer word that merely starts with it")
	}
	if sameOrgToken("rate", "ratepay") {
		t.Error("a word matched a longer word that merely starts with it")
	}
	// One rune apart is what a transliteration costs, and must still meet.
	if !sameOrgToken("rohm", "roehm") {
		t.Error("a one-rune transliteration no longer meets")
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
