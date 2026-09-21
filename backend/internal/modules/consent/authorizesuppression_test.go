// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The suppression rules are a pure predicate over two values, so they are
// asked here rather than through a database. The DB-backed half — that
// decideOne actually applies them in the right order — lives in
// authorizesuppression_integration_test.go.

import (
	"os"
	"regexp"
	"slices"
	"strings"
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

// TestABasisSurvivesAStopThatDoesNotReachTheSend covers suppressionBindsAny,
// which decides whether an allowed send records the ground it relied on.
//
// Two ways a live stop can fail to reach a message, and both must leave the
// basis written: a category it does not bind, and — since the purpose column —
// a marketing send that is not the one a narrow row names. The third case is
// the send the evidence arms resolve, which names no purpose at all: a narrow
// row must not catch it, or a reply the subject themselves started goes out
// with no record of what allowed it.
func TestABasisSurvivesAStopThatDoesNotReachTheSend(t *testing.T) {
	pressed, other := ids.NewV7(), ids.NewV7()
	narrow := []liveStop{{Kind: commsauthz.ReasonObjection, PurposeID: &pressed}}

	if !suppressionBindsAny(narrow, commsauthz.CategoryMarketing, &pressed) {
		t.Error("a narrow stop did not bind its own purpose — the send it was pressed " +
			"against would record a ground and go out")
	}
	if suppressionBindsAny(narrow, commsauthz.CategoryMarketing, &other) {
		t.Error("a narrow stop bound a different marketing purpose")
	}
	if suppressionBindsAny(narrow, commsauthz.CategoryMarketing, nil) {
		t.Error("a narrow stop bound a send that resolved no purpose — the evidence arms " +
			"name none, and withholding their basis is the Art. 15 gap this write closes")
	}
	if suppressionBindsAny(narrow, commsauthz.CategoryInvoiceOrPayment, &pressed) {
		t.Error("an objection bound an invoice — the category test does its own work here")
	}
	if suppressionBindsAny(nil, commsauthz.CategoryMarketing, &pressed) {
		t.Error("no live stop at all was read as binding")
	}
}

// TestTheEarlyExitAgreesWithTheRule holds the two spellings together.
//
// bindsEveryCategory lets decideOne answer without resolving the record, which
// is only sound when the kind stops every category. A restatement is how the
// two drift, so the implication is asserted rather than trusted.
//
// AT EVERY PURPOSE PAIRING, not only at the unscoped one. The early exit reads
// stopKinds, which drops the purpose scoping, so it cannot tell a narrow row
// from a broad one — sound only while no absolute kind consults a purpose.
// Today none does: suppressionBinds compares purposes in the objection arm
// alone, and objection is one of the scoped kinds the early exit declines. The
// day a second kind starts carrying a purpose, the shortcut would deny a send
// the rule would have let through, and nothing else in this package would
// notice. So the pairings are asked here rather than argued in a comment.
func TestTheEarlyExitAgreesWithTheRule(t *testing.T) {
	rowPurpose, sendPurpose := ids.NewV7(), ids.NewV7()
	purposePairings := []struct {
		name      string
		row, send *ids.UUID
	}{
		{"neither names a purpose", nil, nil},
		{"a narrow row against a send that resolved none", &rowPurpose, nil},
		{"a broad row against a send that resolved one", nil, &sendPurpose},
		{"a narrow row against its own purpose", &rowPurpose, &rowPurpose},
		{"a narrow row against a different purpose", &rowPurpose, &sendPurpose},
	}
	for _, kind := range []string{
		commsauthz.ReasonObjection, commsauthz.ReasonRestricted,
		commsauthz.ReasonHardBounce, "a_code_nobody_added_here",
	} {
		if _, absolute := bindsEveryCategory([]string{kind}); !absolute {
			continue
		}
		for _, c := range commsauthz.Categories() {
			for _, pair := range purposePairings {
				if !suppressionBinds(kind, c, pair.row, pair.send) {
					t.Errorf("%s skips resolution but does not bind %s with %s: a message in "+
						"that category is refused without the engine ever working out what it is",
						kind, c, pair.name)
				}
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

// headCatalog is the migrated schema, which owns the kind vocabulary this file
// must be able to read back.
const headCatalog = "../../../migrations/testdata/head_catalog.txt"

// suppressionKindCheck lifts the stored kinds out of the table's own CHECK, so
// the census below cannot fall short of what the column can actually hold: a
// hand-kept list would have stayed green through a fifth kind being added, and
// under-recognition here reports PASS with nothing failing.
var suppressionKindCheck = regexp.MustCompile(
	`communication_suppression_kind CHECK \(\(kind = ANY \(ARRAY\[(.+?)\]\)\)\)`)

// TestEveryStoredKindReachesANamedArmOfTheRule holds the two vocabularies apart
// on purpose, and the mapping between them honest.
//
// The COLUMN stores a kind and the RULE answers about a reason code, and for
// one of the four the spellings differ — 'processing_restriction' in the table
// is ReasonRestricted, 'processing_restricted', to suppressionBinds. Every
// reader of the column therefore owes the row a trip through
// reasonForSuppressionKind, and the cost of forgetting is silent in the worst
// direction: the raw kind matches no case, falls to the unrecognised default,
// and binds EVERY category — including the three Art. 12(3)/13/14/34 oblige us
// to send, which the same rule spares when asked with the mapped code.
//
// So the assertion is that no kind the CHECK admits lands on that default once
// mapped. Derived from the catalog rather than listed here, because a list is
// the thing that stops matching.
func TestEveryStoredKindReachesANamedArmOfTheRule(t *testing.T) {
	catalog, err := os.ReadFile(headCatalog)
	if err != nil {
		t.Fatalf("reading the migrated schema: %v", err)
	}
	found := suppressionKindCheck.FindSubmatch(catalog)
	if found == nil {
		t.Fatal("no communication_suppression kind CHECK in the head catalog — the vocabulary " +
			"this census reads has moved, and an empty one would pass while proving nothing")
	}
	kinds := regexp.MustCompile(`'([a-z_]+)'::text`).FindAllStringSubmatch(string(found[1]), -1)
	if len(kinds) < 4 {
		t.Fatalf("read %d stored kinds out of the CHECK, want the four the column holds — a short "+
			"read is how this passes without asking anything", len(kinds))
	}
	for _, k := range kinds {
		stored := k[1]
		reason := reasonForSuppressionKind(stored)
		if strings.HasPrefix(reason, "unrecognised_suppression:") {
			t.Errorf("stored kind %q maps to %q: the column holds a kind the rule cannot name, so "+
				"every reader of it binds on the default", stored, reason)
			continue
		}
		// The mapped code must reach a NAMED arm. Asked with the raw kind
		// instead, an unrecognised code binds every category — which is what
		// the default does and what this is here to tell apart.
		named := false
		for _, c := range commsauthz.Categories() {
			if !suppressionBinds(reason, c, nil, nil) {
				named = true
				break
			}
		}
		if !named && reason != commsauthz.ReasonHardBounce {
			t.Errorf("stored kind %q (%q) binds every category — only a hard bounce and an "+
				"unrecognised code may do that, so this one is reaching the default", stored, reason)
		}
	}
}

// TestAnAllowedDecisionRecordsOnlyAStopItCanExplain holds the field
// applySuppression stamps when nothing binds.
//
// communication_decision stores kind and resolved_category and no purpose, and
// privacy/sarcommunication.go exports all three to the subject. A BROAD stop
// beside an allowed send of another category explains itself from those two
// columns. A NARROW stop beside an allowed marketing send does not: it reads as
// "objected to marketing, sent marketing", with the purpose that makes it
// correct absent from the row.
//
// The combination could not arise before purpose_id — an objection always bound
// a marketing send — so nothing caught it when it became ordinary.
func TestAnAllowedDecisionRecordsOnlyAStopItCanExplain(t *testing.T) {
	left, other := ids.NewV7(), ids.NewV7()
	narrow := liveStop{Kind: commsauthz.ReasonObjection, PurposeID: &left}
	broad := liveStop{Kind: commsauthz.ReasonObjection}

	// A marketing send for a purpose the narrow stop does not name: allowed,
	// and the row must not claim an objection stood against it.
	allowed := applySuppression(
		commsauthz.Decision{Verdict: commsauthz.VerdictAllow, Resolved: commsauthz.CategoryMarketing},
		[]liveStop{narrow}, &other)
	if allowed.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q, want allow — the narrow stop names another purpose", allowed.Verdict)
	}
	if allowed.Suppression != "" {
		t.Errorf("the decision records suppression=%q beside resolved_category=marketing and "+
			"verdict=allow — the subject's Art. 15 export reads that as knowingly mailing "+
			"somebody who objected, and no column on the row says which list they left",
			allowed.Suppression)
	}

	// A broad stop that does not reach this category still records, because
	// kind and category together say why it did not bind.
	invoice := applySuppression(
		commsauthz.Decision{Verdict: commsauthz.VerdictAllow, Resolved: commsauthz.CategoryInvoiceOrPayment},
		[]liveStop{broad}, nil)
	if invoice.Suppression != commsauthz.ReasonObjection {
		t.Errorf("a broad objection standing beside an allowed invoice recorded %q, want the "+
			"objection — a stop that stood while mail went out is what a later reader needs",
			invoice.Suppression)
	}
}
