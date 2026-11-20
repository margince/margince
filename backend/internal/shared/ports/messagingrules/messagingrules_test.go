// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingrules

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// registerForTest registers r for one test and removes it after, so the
// package's tests stay repeatable within one process (-count).
func registerForTest(t *testing.T, r Rules) {
	t.Helper()
	Register(r)
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		delete(rules, r.Jurisdiction)
	})
}

// Two jurisdictions apply to one message when the sender and the recipient sit
// in different countries, and the answer is never "pick one". A message lawful
// where the sender sits and unlawful where the recipient does is unlawful, so
// each obligation resolves to whichever position permits less.

// The shorter window wins, and a jurisdiction that declares none does not
// shorten anything to zero.
func TestTheShorterWindowWins(t *testing.T) {
	strict := Rules{Jurisdiction: "aa", Version: 1, ReplyWindow: 30 * 24 * time.Hour}
	loose := Rules{Jurisdiction: "bb", Version: 1, ReplyWindow: 365 * 24 * time.Hour}

	if got := stricter(loose, strict).ReplyWindow; got != strict.ReplyWindow {
		t.Errorf("reply window = %s, want the shorter %s", got, strict.ReplyWindow)
	}
	// Zero means "this jurisdiction says nothing", not "expires instantly".
	silent := Rules{Jurisdiction: "cc", Version: 1}
	if got := stricter(strict, silent).ReplyWindow; got != strict.ReplyWindow {
		t.Errorf("reply window = %s; a silent jurisdiction collapsed a real window to nothing", got)
	}
}

// An exception is available only where EVERY applicable jurisdiction grants
// it. A route to advertising without consent that one country does not
// recognise is not available in a send both govern — this is the fold's most
// important direction, because getting it backwards makes the laxest country
// the one that decides.
func TestAnExceptionSurvivesOnlyWhereBothGrantIt(t *testing.T) {
	grants := Rules{Jurisdiction: "aa", Version: 1, MarketingExceptions: []MarketingException{{
		Kind: ExistingCustomer, RequiresSaleEvidence: true,
	}}}
	withholds := Rules{Jurisdiction: "bb", Version: 1}

	if got := stricter(grants, withholds).MarketingExceptions; len(got) != 0 {
		t.Errorf("an exception survived a jurisdiction that does not grant it: %+v", got)
	}
	if got := stricter(withholds, grants).MarketingExceptions; len(got) != 0 {
		t.Error("the fold is order-dependent — an exception survived in one direction only")
	}
}

// A kept exception carries the union of the conditions either side attaches.
// Dropping a condition because the other country does not require it would
// apply the exception on looser terms than one of the two allows.
func TestAKeptExceptionCarriesEveryConditionEitherSideAttaches(t *testing.T) {
	sale := Rules{Jurisdiction: "aa", Version: 1, MarketingExceptions: []MarketingException{{
		Kind: ExistingCustomer, RequiresSaleEvidence: true,
	}}}
	similar := Rules{Jurisdiction: "bb", Version: 1, MarketingExceptions: []MarketingException{{
		Kind: ExistingCustomer, RequiresSimilarity: true, RequiresNoObjection: true,
	}}}

	got := stricter(sale, similar).MarketingExceptions
	if len(got) != 1 {
		t.Fatalf("%d exceptions survived, want 1", len(got))
	}
	e := got[0]
	if !e.RequiresSaleEvidence || !e.RequiresSimilarity || !e.RequiresNoObjection {
		t.Errorf("a condition was dropped in the fold: %+v", e)
	}
}

// Every disclosure either side requires. An obligation one country imposes is
// not lifted by another that does not.
func TestDisclosuresAccumulate(t *testing.T) {
	a := Rules{Jurisdiction: "aa", Version: 1, Disclosures: []Disclosure{{Kind: ControllerIdentity}}}
	b := Rules{Jurisdiction: "bb", Version: 1, Disclosures: []Disclosure{{Kind: AdvertiserContact}}}

	got := stricter(a, b).Disclosures
	if len(got) != 2 {
		t.Fatalf("%d disclosures survived, want both", len(got))
	}
}

// The tighter cap wins. Three per 24h is tighter than ten per 24h and also
// tighter than four per hour, which lets more through in every stretch.
func TestTheTighterCapWinsByRate(t *testing.T) {
	threePerDay := &FrequencyCap{Messages: 3, Window: 24 * time.Hour}
	tenPerDay := &FrequencyCap{Messages: 10, Window: 24 * time.Hour}
	fourPerHour := &FrequencyCap{Messages: 4, Window: time.Hour}

	if got := tighterCap(tenPerDay, threePerDay); got != threePerDay {
		t.Error("the looser of two same-window caps won")
	}
	if got := tighterCap(fourPerHour, threePerDay); got != threePerDay {
		t.Error("a high-rate short-window cap beat a genuinely tighter one")
	}
	// Nil is uncapped, so the other side's cap survives.
	if got := tighterCap(nil, threePerDay); got != threePerDay {
		t.Error("an uncapped jurisdiction erased another's cap")
	}
}

// An acknowledgement owed anywhere is owed. It is a message TO the subject, so
// a country that does not require it is not harmed by one being sent.
func TestAnAcknowledgementOwedAnywhereIsOwed(t *testing.T) {
	owes := Rules{Jurisdiction: "aa", Version: 1, OptOutAcknowledgement: true}
	silent := Rules{Jurisdiction: "bb", Version: 1}
	if !stricter(silent, owes).OptOutAcknowledgement {
		t.Error("an acknowledgement obligation was dropped in the fold")
	}
}

// The fold is not any one jurisdiction's, and must not be stamped as one. A
// decision recording a folded set under a single country's code and version
// would misname the rules it was taken under.
func TestAFoldedSetClaimsNoSingleJurisdiction(t *testing.T) {
	a := Rules{Jurisdiction: "aa", Version: 3}
	b := Rules{Jurisdiction: "bb", Version: 7}
	got := stricter(a, b)
	if got.Jurisdiction != "" {
		t.Errorf("the fold claims jurisdiction %q", got.Jurisdiction)
	}
	if got.Version != 0 {
		t.Errorf("the fold claims version %d, which belongs to one country's set", got.Version)
	}
}

// A fold carries no one pack's review, citations or digest. Each describes the
// source of a single pack. A citation shown under the fold would name one
// country's law for a send two countries govern.
func TestAFoldCarriesNoSinglePacksProvenance(t *testing.T) {
	reviewed := Provenance{ReviewedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Reviewer: "a"}
	a := Rules{
		Jurisdiction: "aa", Version: 3, Provenance: reviewed,
		Citations: []Citation{{Code: "frequency_cap_reached", Instrument: "Decree 1/2020"}},
		Digest:    strings.Repeat("a", 64),
	}
	b := Rules{Jurisdiction: "bb", Version: 7}
	got := stricter(a, b)
	if got.Provenance != (Provenance{}) || got.Citations != nil || got.Digest != "" {
		t.Errorf("the fold kept one pack's provenance: %+v %+v %q", got.Provenance, got.Citations, got.Digest)
	}
}

// A jurisdiction the binary was not compiled with contributes nothing, and
// Strictest reports that it found nothing rather than returning an empty set
// that reads as "no obligations".
func TestAnUnknownJurisdictionIsNotAnAnswer(t *testing.T) {
	_, applied, ok := Strictest("zz")
	if ok {
		t.Error("an unknown jurisdiction reported rules — the caller would read the zero set as 'nothing required'")
	}
	if len(applied.Unknown) != 1 {
		t.Errorf("unknown codes = %v, want the one that could not be resolved", applied.Unknown)
	}
	if len(applied.Codes) != 0 {
		t.Errorf("applied codes = %v; an unknown jurisdiction bound nothing and must not be "+
			"recorded as though it judged the message", applied.Codes)
	}
}

// The registry hands out copies, not its own slices.
//
// Rules is a struct of slices and a pointer, so a plain return aliases the
// stored value. A caller that sorted or filtered what it got — an ordinary
// thing to write — would rewrite the registered rules for every later send in
// the process, silently dropping a statutory condition with nothing failing.
func TestTheRegistryHandsOutCopies(t *testing.T) {
	registerForTest(t, Rules{
		Jurisdiction: "qa", Version: 1,
		MarketingExceptions: []MarketingException{{Kind: ExistingCustomer, RequiresSimilarity: true}},
		FrequencyCap:        &FrequencyCap{Messages: 3, Window: 24 * time.Hour},
		Instruments:         []Instrument{{Name: "Decree 1/2020", SourceURL: "https://example.org/decree"}},
		Provenance: Provenance{
			SourceCheckedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ReviewedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Reviewer: "a",
		},
		Citations: []Citation{{Code: "frequency_cap_reached", Instrument: "Decree 1/2020", Provision: "Art. 1"}},
	})

	got, ok := For("qa")
	if !ok {
		t.Fatal("the rules did not register")
	}
	got.MarketingExceptions[0].RequiresSimilarity = false
	got.FrequencyCap.Messages = 1000
	got.Instruments[0].Name = "something else entirely"
	got.Citations[0].Provision = "Art. 99"

	again, _ := For("qa")
	if !again.MarketingExceptions[0].RequiresSimilarity {
		t.Error("a caller erased a condition from the registry's own copy")
	}
	if again.FrequencyCap.Messages != 3 {
		t.Errorf("a caller rewrote the registered cap to %d", again.FrequencyCap.Messages)
	}
	// The citation a decision is read back against. A caller rewriting it would
	// leave every later decision recorded under a law the pack never stated.
	if again.Instruments[0].Name != "Decree 1/2020" {
		t.Errorf("a caller rewrote the registered citation to %q", again.Instruments[0].Name)
	}
	if again.Citations[0].Provision != "Art. 1" {
		t.Errorf("a caller rewrote the registered provision to %q", again.Citations[0].Provision)
	}
}

// Registering is a copy too, so a caller that keeps its literal and mutates it
// afterwards does not reach in.
func TestRegisteringTakesACopy(t *testing.T) {
	exceptions := []MarketingException{{Kind: ExistingCustomer, RequiresSaleEvidence: true}}
	instruments := []Instrument{{Name: "Decree 1/2020"}}
	registerForTest(t, Rules{
		Jurisdiction: "qb", Version: 1,
		MarketingExceptions: exceptions,
		Instruments:         instruments,
	})

	exceptions[0].RequiresSaleEvidence = false
	instruments[0].Name = "something else entirely"

	got, _ := For("qb")
	if !got.MarketingExceptions[0].RequiresSaleEvidence {
		t.Error("mutating the caller's own slice reached into the registry")
	}
	if got.Instruments[0].Name != "Decree 1/2020" {
		t.Errorf("mutating the caller's own citation reached into the registry: %q",
			got.Instruments[0].Name)
	}
}

// The fold may never permit more than an input permits.
//
// This is the counterexample that proved it could: a set naming one kind twice,
// where the weak duplicate overwrote the strong entry. The fold then required
// none of the three conditions the first entry demanded — more permissive than
// either side, which is the one thing a strictest-wins fold may not be.
func TestADuplicateKindCannotErodeAnExceptionsConditions(t *testing.T) {
	strong := MarketingException{
		Kind: ExistingCustomer, RequiresSaleEvidence: true,
		RequiresCollectionTimeOptOut: true, RequiresSimilarity: true, RequiresNoObjection: true,
	}
	weak := MarketingException{Kind: ExistingCustomer, RequiresNoObjection: true}

	a := Rules{Jurisdiction: "aa", Version: 1, MarketingExceptions: []MarketingException{strong, weak}}
	b := Rules{Jurisdiction: "bb", Version: 1, MarketingExceptions: []MarketingException{weak}}

	got := stricter(a, b).MarketingExceptions
	if len(got) != 1 {
		t.Fatalf("%d exceptions survived, want 1 — a duplicate kind produced an order-dependent answer", len(got))
	}
	e := got[0]
	if !e.RequiresSaleEvidence || !e.RequiresCollectionTimeOptOut || !e.RequiresSimilarity {
		t.Errorf("the fold permits more than its input did: %+v", e)
	}
}

// And the set that made it possible is refused outright.
func TestARuleSetCannotNameOneExceptionKindTwice(t *testing.T) {
	err := Rules{Jurisdiction: "aa", Version: 1, MarketingExceptions: []MarketingException{
		{Kind: ExistingCustomer, RequiresSaleEvidence: true},
		{Kind: ExistingCustomer, RequiresNoObjection: true},
	}}.Validate()
	if err == nil {
		t.Error("a rule set naming one exception kind twice was accepted")
	}
}

// A cap that cannot bind as written is the strictest thing there is, and it
// does not depend on which side it sits on.
//
// A zero window over a zero count is NaN, which loses every comparison — so the
// genuinely tighter cap lost whenever the degenerate one sat on the left, and
// the fold's answer depended on the order the caller listed the countries in.
func TestADegenerateCapDoesNotWinByArgumentOrder(t *testing.T) {
	broken := &FrequencyCap{}
	binding := &FrequencyCap{Messages: 1, Window: 24 * time.Hour}

	if tighterCap(broken, binding) != broken || tighterCap(binding, broken) != broken {
		t.Error("a cap that cannot bind lost to a real one, or won only from one side")
	}
}

// A cap is tighter only if it lets no more through in every stretch of time.
// Two per 3h has the lower average and lets two out at once. One per hour lets
// one out at once and more over a day. Neither holds the other, so the fold
// answers a cap that holds both, whichever side each is on.
func TestACapWithTheLowerRateAndTheBiggerBurstDoesNotWin(t *testing.T) {
	hourly := &FrequencyCap{Messages: 1, Window: time.Hour}
	burst := &FrequencyCap{Messages: 2, Window: 3 * time.Hour}
	want := FrequencyCap{Messages: 1, Window: 3 * time.Hour}
	for _, got := range []*FrequencyCap{tighterCap(hourly, burst), tighterCap(burst, hourly)} {
		if *got != want {
			t.Errorf("the fold of 1/1h and 2/3h is %+v, want %+v", *got, want)
		}
	}
	// Two per hour against three per 1h24m: lower burst, lower rate, and
	// still four through just after the first hour against three.
	fast := &FrequencyCap{Messages: 2, Window: time.Hour}
	slow := &FrequencyCap{Messages: 3, Window: 84 * time.Minute}
	if got := tighterCap(fast, slow); got == fast || got == slow {
		t.Errorf("one of 2/1h and 3/84m won although neither holds the other: %+v", *got)
	}
	// A cap that holds the other in every stretch still wins outright.
	daily := &FrequencyCap{Messages: 1, Window: 24 * time.Hour}
	if tighterCap(hourly, daily) != daily || tighterCap(daily, hourly) != daily {
		t.Error("one per day lost to one per hour")
	}
}

// Equal rates are not equal burst: {1, 1h} and {24, 24h} permit the same
// average, and a 24-message burst is not the same restriction as one an hour.
func TestEqualRatesBreakTowardTheSmallerBurst(t *testing.T) {
	hourly := &FrequencyCap{Messages: 1, Window: time.Hour}
	daily := &FrequencyCap{Messages: 24, Window: 24 * time.Hour}

	if tighterCap(daily, hourly) != hourly || tighterCap(hourly, daily) != hourly {
		t.Error("a 24-message burst beat a one-per-hour throttle at the same rate")
	}
}

// Strictest must not report a complete answer over a partial fold.
//
// The intended call is Strictest(sender, recipient). With the German pack
// compiled in and a recipient whose country has none, a silent skip returned
// ok=true carrying Germany's existing-customer exception — applied to somebody
// whose law the process knows nothing about, which is exactly the
// laxest-country-decides failure the fold exists to prevent.
func TestStrictestReportsTheJurisdictionsItCouldNotResolve(t *testing.T) {
	registerForTest(t, Rules{
		Jurisdiction: "qc", Version: 1,
		MarketingExceptions: []MarketingException{{Kind: ExistingCustomer, RequiresSaleEvidence: true}},
	})

	got, applied, ok := Strictest("qc", "zz")
	if !ok {
		t.Fatal("a known jurisdiction reported nothing")
	}
	if len(applied.Unknown) != 1 || applied.Unknown[0] != "zz" {
		t.Fatalf("unknown = %v, want the code that could not be resolved", applied.Unknown)
	}
	// The unresolvable code is NOT an applied one. A decision recording it
	// would name a jurisdiction that bound nothing.
	if len(applied.Codes) != 1 || applied.Codes[0] != "qc" {
		t.Fatalf("applied = %v, want only the jurisdiction that contributed", applied.Codes)
	}
	if applied.Folded() {
		t.Error("one contributing jurisdiction reported as a fold — Rules.Version is " +
			"meaningful here and would be discarded")
	}
	// The rules still come back — the caller decides what to do about a partial
	// answer — but it can no longer mistake one for a complete fold.
	if len(got.MarketingExceptions) != 1 {
		t.Error("the resolvable jurisdiction's rules were lost")
	}
}

// TestAFoldNamesTheJurisdictionsItUsedBecauseTheVersionCannot.
//
// stricter() zeroes Jurisdiction and Version whenever two countries fold: a
// fold is nobody's rule set, and stamping it with one country's number would
// misname the rules a decision was taken under. That left a folded decision
// with nothing to say about what judged it. The applied codes are the answer,
// and they are the reason Strictest returns them at all.
func TestAFoldNamesTheJurisdictionsItUsedBecauseTheVersionCannot(t *testing.T) {
	registerForTest(t, Rules{Jurisdiction: "fa", Version: 7, ReplyWindow: 30 * 24 * time.Hour})
	registerForTest(t, Rules{Jurisdiction: "fb", Version: 9, ReplyWindow: 365 * 24 * time.Hour})

	got, applied, ok := Strictest("fa", "fb")
	if !ok {
		t.Fatal("two known jurisdictions reported nothing")
	}
	if got.Version != 0 || got.Jurisdiction != "" {
		t.Errorf("the fold kept version %d and jurisdiction %q — it belongs to neither",
			got.Version, got.Jurisdiction)
	}
	if !applied.Folded() {
		t.Error("two contributing jurisdictions did not report as a fold")
	}
	if len(applied.Codes) != 2 || applied.Codes[0] != "fa" || applied.Codes[1] != "fb" {
		t.Errorf("applied = %v, want both contributing jurisdictions in the order asked — "+
			"this is the only record of which rules judged a folded decision", applied.Codes)
	}
	// And the fold is still the stricter answer, so the codes are not a
	// consolation prize for having lost the rules.
	if got.ReplyWindow != 30*24*time.Hour {
		t.Errorf("reply window = %s, want the shorter one", got.ReplyWindow)
	}
}

// A fold keeps every advertising content rule either side declares: the
// refusal layout, the foreign-advertiser notice and every restricted claim.
func TestAdvertisingContentRulesSurviveTheFold(t *testing.T) {
	a := Rules{Jurisdiction: "aa", Version: 1, ClaimsNeedProof: []string{"best"}}
	b := Rules{
		Jurisdiction: "bb", Version: 1, RefusalSectionLast: true, ForeignAdvertiserNeedsLocalAgency: true,
		ClaimsNeedProof: []string{"best", "tốt nhất"},
	}

	got := stricter(a, b)
	if !got.RefusalSectionLast {
		t.Error("the fold dropped the refusal layout")
	}
	if !got.ForeignAdvertiserNeedsLocalAgency {
		t.Error("the fold dropped the foreign-advertiser notice")
	}
	if !slices.Equal(got.ClaimsNeedProof, []string{"best", "tốt nhất"}) {
		t.Errorf("claims %v, want each restricted word once", got.ClaimsNeedProof)
	}
	if !slices.Equal(a.ClaimsNeedProof, []string{"best"}) {
		t.Errorf("the fold wrote into its input: %v", a.ClaimsNeedProof)
	}
}

// A caller that edits the means it was handed must not edit the registered
// rule for every later send.
func TestTheRegistryHandsOutItsOwnCopyOfTheMeans(t *testing.T) {
	r := Rules{Disclosures: []Disclosure{{Kind: ObjectionRoute, Means: []ContactMeans{MeansWeb}}}}
	copied := clone(r)
	copied.Disclosures[0].Means[0] = MeansPhone
	if r.Disclosures[0].Means[0] != MeansWeb {
		t.Error("the clone shares its means with the registered rule set")
	}
}

// A data-protection law that follows the recipient, and one with no
// legitimate-interest basis, bind a send whichever side declares them.
func TestARecipientBoundLawSurvivesTheFold(t *testing.T) {
	bound := Rules{
		Jurisdiction: "aa", Version: 1, DataProtectionFollowsRecipient: true,
		NoLegitimateInterests: true, DisclosureLanguage: "vi",
	}
	plain := Rules{Jurisdiction: "bb", Version: 1}
	for _, folded := range []Rules{stricter(plain, bound), stricter(bound, plain)} {
		if !folded.DataProtectionFollowsRecipient || !folded.NoLegitimateInterests || folded.DisclosureLanguage != "vi" {
			t.Errorf("fold = %+v, want the recipient-bound law and its language kept", folded)
		}
	}
}

// The request rule of a fold covers each class either side covers, and every
// class when one side covers every class.
func TestTheRequestRuleSurvivesTheFold(t *testing.T) {
	narrow := Rules{
		Jurisdiction: "aa", Version: 1, ConsentConfirmationNeedsRequest: true,
		ConsentRequestFirstFor: []PurposeClass{"marketing"},
	}
	none := Rules{Jurisdiction: "bb", Version: 1}
	if got := stricter(none, narrow); !slices.Equal(got.ConsentRequestFirstFor, []PurposeClass{"marketing"}) {
		t.Errorf("classes %v, want the one side's marketing", got.ConsentRequestFirstFor)
	}
	every := Rules{Jurisdiction: "cc", Version: 1, ConsentConfirmationNeedsRequest: true}
	if got := stricter(narrow, every); len(got.ConsentRequestFirstFor) != 0 || !got.ConsentConfirmationNeedsRequest {
		t.Errorf("classes %v, want every class: one side asks a request for each", got.ConsentRequestFirstFor)
	}
}

// One subject begins with one label, so Rules.SubjectPrefix holds the first.
// Every distinct label stays owed in Applied.SubjectPrefixes.
func TestTheFoldKeepsEveryDistinctSubjectLabel(t *testing.T) {
	Register(Rules{Jurisdiction: "pa", Version: 1, SubjectPrefix: "(광고)"})
	Register(Rules{Jurisdiction: "pb", Version: 1, SubjectPrefix: "<ADV>"})
	Register(Rules{Jurisdiction: "pc", Version: 1, SubjectPrefix: "<adv>"})
	Register(Rules{Jurisdiction: "pd", Version: 1})
	for _, tc := range []struct {
		codes []jurisdiction.Code
		first string
		want  []string
	}{
		{[]jurisdiction.Code{"pa"}, "(광고)", []string{"(광고)"}},
		{[]jurisdiction.Code{"pd", "pa"}, "(광고)", []string{"(광고)"}},
		{[]jurisdiction.Code{"pb", "pc"}, "<ADV>", []string{"<ADV>"}},
		{[]jurisdiction.Code{"pa", "pd", "pb"}, "(광고)", []string{"(광고)", "<ADV>"}},
		{[]jurisdiction.Code{"pd"}, "", nil},
	} {
		rules, applied, _ := Strictest(tc.codes...)
		if rules.SubjectPrefix != tc.first || !slices.Equal(applied.SubjectPrefixes, tc.want) {
			t.Errorf("%v: prefix %q and labels %q, want %q and %q",
				tc.codes, rules.SubjectPrefix, applied.SubjectPrefixes, tc.first, tc.want)
		}
	}
}
