// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The suppression rules are a pure predicate over two values, so they are
// asked here rather than through a database. The DB-backed half — that
// decideOne actually applies them in the right order — lives in
// authorizesuppression_integration_test.go.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// serviceCategories is the five, written out. Naming them here rather than
// asking ServesTheSubject is the point: an expectation computed the way the
// code computes it agrees with a wrong rule.
var serviceCategories = []commsauthz.Category{
	commsauthz.CategorySecurityNotice, commsauthz.CategoryPrivacyNotice,
	commsauthz.CategoryRecordConfirmation, commsauthz.CategoryConsentConfirmation,
	commsauthz.CategoryOptoutConfirmation,
}

// survivesArt18 is the THREE a statutory restriction leaves room for — fewer
// than the five above, and the difference is the point: a record confirmation
// and a consent confirmation are messages Art. 18(2) offers no gateway for.
var survivesArt18 = []commsauthz.Category{
	commsauthz.CategorySecurityNotice, commsauthz.CategoryPrivacyNotice,
	commsauthz.CategoryOptoutConfirmation,
}

// everythingButTheFive lists the categories that are not in serviceCategories,
// also written out.
//
// Held by: TestWhatEachSuppressionBinds, whose first assertion is that these
// two lists together are exactly the vocabulary — so a category added to
// commsauthz and to neither list fails rather than being silently unjudged.
var everythingButTheFive = []commsauthz.Category{
	commsauthz.CategoryReplyToInbound, commsauthz.CategoryRequestedFollowup,
	commsauthz.CategoryPrecontractQuote, commsauthz.CategoryActiveDealFollowup,
	commsauthz.CategoryCustomerService, commsauthz.CategoryAccountNotice,
	commsauthz.CategoryContractNotice, commsauthz.CategoryInvoiceOrPayment,
	commsauthz.CategoryMarketing,
}

// TestWhatEachSuppressionBinds spells out every answer.
func TestWhatEachSuppressionBinds(t *testing.T) {
	all := append(append([]commsauthz.Category{}, everythingButTheFive...), serviceCategories...)
	if len(all) != len(commsauthz.Categories()) {
		t.Fatalf("this table names %d categories and the vocabulary holds %d — a category was added "+
			"without deciding which suppressions reach it", len(all), len(commsauthz.Categories()))
	}

	// What a restriction stops: everything except the three Art. 18(2) spares.
	restricted := []commsauthz.Category{}
	for _, c := range all {
		if !slices.Contains(survivesArt18, c) {
			restricted = append(restricted, c)
		}
	}

	bound := map[string][]commsauthz.Category{
		commsauthz.ReasonObjection:  {commsauthz.CategoryMarketing},
		commsauthz.ReasonRestricted: restricted,
		// The five included: no template makes a dead address live.
		commsauthz.ReasonHardBounce: all,
		// The subject said stop, so nothing goes EXCEPT the three the
		// controller owes them whatever they want sent — the same three
		// Art. 18(2) spares. Binding all fourteen meant a contact who asked
		// us to stop never received the confirmation that we had stopped.
		commsauthz.ReasonSubjectRequest: restricted,
		// A reason code this function does not recognise refuses everything.
		"a_code_nobody_added_here": all,
	}
	for kind, categories := range bound {
		for _, c := range all {
			want := slices.Contains(categories, c)
			// nil, nil: a broad row (no purpose_id) against a send with no
			// resolved purpose — the case every existing row was, and still is,
			// before a writer ever narrows one.
			if got := suppressionBinds(kind, c, nil, nil); got != want {
				t.Errorf("%s against %s: binds = %v, want %v", kind, c, got, want)
			}
		}
	}
}

// TestAPurposeScopedObjectionBindsOnlyItsPurpose covers the four cases the
// purpose column adds: broad-vs-narrow row, crossed against a send with and
// without its own resolved purpose. A narrow row must narrow, and a
// purpose-less send (the evidence arms) must never be caught by one — see
// suppressionBinds's doc comment for why.
func TestAPurposeScopedObjectionBindsOnlyItsPurpose(t *testing.T) {
	p1, p2 := ids.NewV7(), ids.NewV7()
	// broad row (nil purpose) binds all marketing, as today
	if !suppressionBinds(commsauthz.ReasonObjection, commsauthz.CategoryMarketing, nil, &p1) {
		t.Fatal("a broad objection must bind a marketing send")
	}
	// narrow row binds a send for the SAME purpose
	if !suppressionBinds(commsauthz.ReasonObjection, commsauthz.CategoryMarketing, &p1, &p1) {
		t.Fatal("a narrow objection must bind its own purpose")
	}
	// narrow row does NOT bind a send for a DIFFERENT purpose
	if suppressionBinds(commsauthz.ReasonObjection, commsauthz.CategoryMarketing, &p1, &p2) {
		t.Fatal("a narrow objection must not bind a different purpose")
	}
	// narrow row does NOT bind a purpose-less (evidence-arm) send
	if suppressionBinds(commsauthz.ReasonObjection, commsauthz.CategoryMarketing, &p1, nil) {
		t.Fatal("a narrow objection must not bind a send with no resolved purpose")
	}
}

// TestTheEarlyExitAgreesWithTheRule holds the two spellings together.
//
// bindsEveryCategory lets decideOne answer without resolving the record, which
// is only sound when the kind stops every category. A restatement is how the
// two drift, so the implication is asserted rather than trusted.
func TestTheEarlyExitAgreesWithTheRule(t *testing.T) {
	for _, kind := range []string{
		commsauthz.ReasonObjection, commsauthz.ReasonRestricted,
		commsauthz.ReasonHardBounce, "a_code_nobody_added_here",
	} {
		if _, absolute := bindsEveryCategory([]string{kind}); !absolute {
			continue
		}
		for _, c := range commsauthz.Categories() {
			if !suppressionBinds(kind, c, nil, nil) {
				t.Errorf("%s skips resolution but does not bind %s: a message in that category "+
					"is refused without the engine ever working out what it is", kind, c)
			}
		}
	}
}

// TestEveryLiveKindIsAsked holds the loop in applySuppression.
//
// A contact may carry several suppressions at once, and since reach became
// category-dependent they no longer agree: an objection binds only marketing
// while a hard bounce binds everything. Asking one and stopping — which the
// reader did for years, ordered by a fixed strength — lets the others through.
// Asserted at BOTH orderings so the answer cannot depend on which row the
// planner returned first.
func TestEveryLiveKindIsAsked(t *testing.T) {
	reply := commsauthz.CategoryReplyToInbound
	for _, order := range [][]string{
		{commsauthz.ReasonObjection, commsauthz.ReasonHardBounce},
		{commsauthz.ReasonHardBounce, commsauthz.ReasonObjection},
	} {
		stops := make([]liveStop, len(order))
		for i, kind := range order {
			stops[i] = liveStop{Kind: kind}
		}
		d := applySuppression(
			commsauthz.Decision{Resolved: reply, Verdict: commsauthz.VerdictAllow}, stops, nil)
		if d.Verdict != commsauthz.VerdictDeny {
			t.Errorf("%v against %s: verdict = %q, want deny — the objection does not bind a "+
				"reply, so the hard bounce beside it must be the one that answers",
				order, reply, d.Verdict)
		}
		if d.ReasonCode != commsauthz.ReasonHardBounce {
			t.Errorf("%v: reason = %q, want %q — the BINDING kind answers, not the first read",
				order, d.ReasonCode, commsauthz.ReasonHardBounce)
		}
	}
}
