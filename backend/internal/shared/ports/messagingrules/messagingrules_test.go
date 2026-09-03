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
	if _, ok := Strictest("zz"); ok {
		t.Error("an unknown jurisdiction reported rules — the caller would read the zero set as 'nothing required'")
	}
}
