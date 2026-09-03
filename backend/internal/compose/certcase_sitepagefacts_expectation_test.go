// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What this site's expectation vocabulary means, and what it refuses.
//
// page_facts is the second site whose corpus makes a NEGATIVE claim — that a
// page files no value under a named field — so it carries the same two
// spellings site_extract/profile does: the bare field-to-value map, and an
// explicit grounded/not_grounded mapping.
//
// The negative half is not decoration. The guard it was ported for is itself an
// absence: a customer story's own industry, headquarters and region must not be
// filed as the READING company's market, and a bare map can only pin the one
// fact that SHOULD be extracted. Left to the judge alone, a reply that grounded
// the product AND filed the customer's city as geography reported `accepted` on
// the deterministic outcome, because the rubric band is HardPass independent.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/modules/people"
)

// The explicit form's two keys are the whole vocabulary. A mistyped one would
// otherwise load as an expectation that forbids nothing and pass whatever the
// model filed — the exact failure the negative claim exists to prevent.
func TestSitePageFactsCaseRefusesAnExplicitExpectationWithAnUnknownKey(t *testing.T) {
	_, err := sitePageFactsCases{}.Prepare(sitePageFactsCatalogFixture(t),
		json.RawMessage(`{"grounded":{"service":"Cloud Cost Audit"},"not_ground":["geography"]}`))
	if err == nil {
		t.Fatal("an expectation with a mistyped key prepared")
	}
	if !strings.Contains(err.Error(), "not_grounded") {
		t.Errorf("the refusal does not name the vocabulary it read: %v", err)
	}
}

// A field both required and forbidden is a contradiction no reply can satisfy,
// so it is named at Prepare rather than measured as a permanent disagreement.
func TestSitePageFactsCaseRefusesAFieldItBothExpectsAndForbids(t *testing.T) {
	_, err := sitePageFactsCases{}.Prepare(sitePageFactsCatalogFixture(t),
		json.RawMessage(`{"grounded":{"service":"Cloud Cost Audit"},"not_grounded":["service"]}`))
	if err == nil {
		t.Fatal("an expectation that both requires and forbids one field prepared")
	}
	if !strings.Contains(err.Error(), "no reply can satisfy both") {
		t.Errorf("the refusal does not say why the pair is unmeasurable: %v", err)
	}
}

// Forbidding a field this page's MENU never offers measures nothing: the schema
// enum does not carry it, so no reply could file it however wrong the model is.
func TestSitePageFactsCaseRefusesAForbiddenFieldTheMenuNeverOffers(t *testing.T) {
	_, err := sitePageFactsCases{}.Prepare(sitePageFactsCatalogFixture(t),
		json.RawMessage(`{"grounded":{"service":"Cloud Cost Audit"},"not_grounded":["founded_year"]}`))
	if err == nil {
		t.Fatal("an expectation forbidding a field off the menu prepared")
	}
	if !strings.Contains(err.Error(), "never offers the model") {
		t.Errorf("the refusal does not say the menu never offers it: %v", err)
	}
}

// An expectation that requires nothing and forbids nothing asserts nothing —
// and the explicit form makes that shape writable where the bare map could not.
func TestSitePageFactsCaseRefusesAnExpectationThatAssertsNothing(t *testing.T) {
	_, err := sitePageFactsCases{}.Prepare(sitePageFactsCatalogFixture(t),
		json.RawMessage(`{"grounded":{},"not_grounded":[]}`))
	if err == nil {
		t.Fatal("an expectation asserting nothing prepared")
	}
	if !strings.Contains(err.Error(), "no reply could disagree with it") {
		t.Errorf("the refusal does not say the expectation is empty: %v", err)
	}
}

// Both spellings read as the same claim, which is what keeps the corpus's
// existing bare maps legal rather than rewriting a hundred scenarios for a
// vocabulary most of them do not need.
func TestSitePageFactsCaseReadsBothSpellingsOfOneExpectation(t *testing.T) {
	bare, err := parseGroundedExpectation(sitePageFactsSite, json.RawMessage(`{"service":"Cloud Cost Audit"}`))
	if err != nil {
		t.Fatalf("the bare form did not parse: %v", err)
	}
	explicit, err := parseGroundedExpectation(sitePageFactsSite,
		json.RawMessage(`{"grounded":{"service":"Cloud Cost Audit"}}`))
	if err != nil {
		t.Fatalf("the explicit form did not parse: %v", err)
	}
	if bare.grounded["service"] != explicit.grounded["service"] {
		t.Errorf("the two spellings ground differently: %v vs %v", bare.grounded, explicit.grounded)
	}
	if len(bare.notGrounded) != 0 {
		t.Errorf("the bare form forbids %v, and it can make no negative claim at all", bare.notGrounded)
	}
}

// The whole point: a reply that grounds every expected fact AND files a
// forbidden one is a wrong answer, not the clean run the positive comparison
// alone would call it.
func TestSitePageFactsCaseFailsAReplyThatFilesAForbiddenField(t *testing.T) {
	fixture := sitePageFactsCatalogFixture(t)
	prepared, err := sitePageFactsCases{}.Prepare(fixture,
		json.RawMessage(`{"grounded":{"service":"Cloud Cost Audit"},"not_grounded":["geography"]}`))
	if err != nil {
		t.Fatalf("preparing the case: %v", err)
	}
	// Both claims cite a real passage, so the citation gate keeps both and the
	// verdict is this case's to make rather than the gate's.
	audit := sitePageFactsCatalogID(t, "Cloud Cost Audit")
	reply := `{"facts":[` +
		sitePageFactsClaim(people.FactService, "Cloud Cost Audit", audit) + `,` +
		sitePageFactsClaim(people.FactGeography, "Cloud Cost Audit", audit) + `]}`
	trace, err := prepared.Run(context.Background(), sitePageFactsCompleterStub{reply: reply})
	if err != nil {
		t.Fatalf("running the case: %v", err)
	}

	outcome := prepared.Evaluate(trace)

	if outcome.Result != aitasks.OutcomeWrongAnswer {
		t.Fatalf("a reply filing a forbidden field scored %q, want a wrong answer", outcome.Result)
	}
	if !strings.Contains(outcome.Detail, people.FactGeography) {
		t.Errorf("the detail does not name the field that was filed: %q", outcome.Detail)
	}
}
