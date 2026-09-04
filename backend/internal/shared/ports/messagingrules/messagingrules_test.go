// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messagingrules

import (
	"testing"
	"time"
)

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

// The tighter cap wins, compared as a RATE. Three per 24h is tighter than ten
// per 24h and also tighter than four per hour — comparing raw counts would let
// a high-count short-window cap look looser than it is.
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

// A jurisdiction the binary was not compiled with contributes nothing, and
// Strictest reports that it found nothing rather than returning an empty set
// that reads as "no obligations".
func TestAnUnknownJurisdictionIsNotAnAnswer(t *testing.T) {
	_, unknown, ok := Strictest("zz")
	if ok {
		t.Error("an unknown jurisdiction reported rules — the caller would read the zero set as 'nothing required'")
	}
	if len(unknown) != 1 {
		t.Errorf("unknown codes = %v, want the one that could not be resolved", unknown)
	}
}

// The registry hands out copies, not its own slices.
//
// Rules is a struct of slices and a pointer, so a plain return aliases the
// stored value. A caller that sorted or filtered what it got — an ordinary
// thing to write — would rewrite the registered rules for every later send in
// the process, silently dropping a statutory condition with nothing failing.
func TestTheRegistryHandsOutCopies(t *testing.T) {
	Register(Rules{
		Jurisdiction: "qa", Version: 1,
		MarketingExceptions: []MarketingException{{Kind: ExistingCustomer, RequiresSimilarity: true}},
		FrequencyCap:        &FrequencyCap{Messages: 3, Window: 24 * time.Hour},
	})

	got, ok := For("qa")
	if !ok {
		t.Fatal("the rules did not register")
	}
	got.MarketingExceptions[0].RequiresSimilarity = false
	got.FrequencyCap.Messages = 1000

	again, _ := For("qa")
	if !again.MarketingExceptions[0].RequiresSimilarity {
		t.Error("a caller erased a condition from the registry's own copy")
	}
	if again.FrequencyCap.Messages != 3 {
		t.Errorf("a caller rewrote the registered cap to %d", again.FrequencyCap.Messages)
	}
}

// Registering is a copy too, so a caller that keeps its literal and mutates it
// afterwards does not reach in.
func TestRegisteringTakesACopy(t *testing.T) {
	exceptions := []MarketingException{{Kind: ExistingCustomer, RequiresSaleEvidence: true}}
	Register(Rules{Jurisdiction: "qb", Version: 1, MarketingExceptions: exceptions})

	exceptions[0].RequiresSaleEvidence = false

	got, _ := For("qb")
	if !got.MarketingExceptions[0].RequiresSaleEvidence {
		t.Error("mutating the caller's own slice reached into the registry")
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
	Register(Rules{
		Jurisdiction: "qc", Version: 1,
		MarketingExceptions: []MarketingException{{Kind: ExistingCustomer, RequiresSaleEvidence: true}},
	})

	got, unknown, ok := Strictest("qc", "zz")
	if !ok {
		t.Fatal("a known jurisdiction reported nothing")
	}
	if len(unknown) != 1 || unknown[0] != "zz" {
		t.Fatalf("unknown = %v, want the code that could not be resolved", unknown)
	}
	// The rules still come back — the caller decides what to do about a partial
	// answer — but it can no longer mistake one for a complete fold.
	if len(got.MarketingExceptions) != 1 {
		t.Error("the resolvable jurisdiction's rules were lost")
	}
}
