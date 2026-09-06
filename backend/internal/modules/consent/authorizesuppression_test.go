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
		// The subject said stop, so nothing goes.
		commsauthz.ReasonSubjectRequest: all,
		// A reason code this function does not recognise refuses everything.
		"a_code_nobody_added_here": all,
	}
	for kind, stops := range bound {
		for _, c := range all {
			want := slices.Contains(stops, c)
			if got := suppressionBinds(kind, c); got != want {
				t.Errorf("%s against %s: binds = %v, want %v", kind, c, got, want)
			}
		}
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
			if !suppressionBinds(kind, c) {
				t.Errorf("%s skips resolution but does not bind %s: a message in that category "+
					"is refused without the engine ever working out what it is", kind, c)
			}
		}
	}
}

// TestEveryLiveKindIsAsked holds the loop in applySuppression.
//
// A person may carry several suppressions at once, and since reach became
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
		d := applySuppression(
			commsauthz.Decision{Resolved: reply, Verdict: commsauthz.VerdictAllow}, order)
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
