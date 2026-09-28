// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contract says every hit this surface returns carries `authoritative`,
// and a client author reading that writes code with no branch for the other
// three. The field is nullable and null means UNKNOWN — a weaker statement
// than the one the sentence makes — so a hit that arrived without a title, a
// snippet or a tag count must still carry the tier: those are the shapes a
// conditional tag would have missed, and they are the ordinary ones.
func TestEveryHitCarriesTheAuthoritativeTier(t *testing.T) {
	t.Parallel()
	carried := 7
	hits := []Hit{
		{Type: "contact", ID: ids.NewV7(), Title: "Anna Schuster", Snippet: "…", Score: 0.9},
		{Type: "company", ID: ids.NewV7()},
		{Type: "tag", ID: ids.NewV7(), Title: "prospect", CarriedBy: &carried},
		{Type: "activity", ID: ids.NewV7(), EmailSummary: &crmcontracts.EmailSummary{}},
	}

	for at, result := range wireHits(hits) {
		if result.TrustTier == nil {
			t.Errorf("hit %d (%s) carries no trust tier; null means UNKNOWN, which is not what the contract promises about a native hit",
				at, hits[at].Type)
			continue
		}
		if want := crmcontracts.SearchResultTrustTierSearchResultTrustTierAuthoritative; *result.TrustTier != want {
			t.Errorf("hit %d (%s) carries tier %q, want %q", at, hits[at].Type, *result.TrustTier, want)
		}
	}
}

// An empty page is a page, and a client reads its `data` as a list rather than
// as a null. The contract declares `data` required, so a nil slice would put
// `"data": null` on the wire against it.
func TestAnEmptyPageRendersAnEmptyListRatherThanNull(t *testing.T) {
	t.Parallel()
	if got := wireHits(nil); got == nil {
		t.Error("an empty page rendered a nil slice, which marshals as null against a required array")
	} else if len(got) != 0 {
		t.Errorf("an empty page rendered %d result(s)", len(got))
	}
}

// The optional members are optional: a hit that has none must not put an empty
// string or a zero where the contract says the member is simply absent.
func TestAHitWithoutOptionalMembersOmitsThem(t *testing.T) {
	t.Parallel()
	results := wireHits([]Hit{{Type: "company", ID: ids.NewV7()}})
	if len(results) != 1 {
		t.Fatalf("one hit rendered %d results", len(results))
	}
	bare := results[0]
	if bare.Title != nil {
		t.Errorf("a hit with no title rendered %q", *bare.Title)
	}
	if bare.Snippet != nil {
		t.Errorf("a hit with no snippet rendered %q", *bare.Snippet)
	}
	if bare.CarriedBy != nil {
		t.Errorf("a non-tag hit rendered a carried-by count of %d", *bare.CarriedBy)
	}
	if bare.EmailSummary != nil {
		t.Error("a non-email hit rendered an email summary")
	}
	if bare.IsPartner != nil {
		t.Errorf("a company hit nobody marked rendered is_partner %v, which says the account was "+
			"checked", *bare.IsPartner)
	}
}

// The marker is tri-state on the wire, so a literal `false` has to survive the
// render: a company checked and found to carry no programme is a different
// answer from one nobody checked, and omitting the false collapses the two.
func TestACompanyHitRendersTheLiteralPartnerMarker(t *testing.T) {
	t.Parallel()
	partner, notPartner := true, false
	results := wireHits([]Hit{
		{Type: "company", ID: ids.NewV7(), IsPartner: &partner},
		{Type: "company", ID: ids.NewV7(), IsPartner: &notPartner},
	})
	if len(results) != 2 {
		t.Fatalf("two hits rendered %d results", len(results))
	}
	if results[0].IsPartner == nil || !*results[0].IsPartner {
		t.Errorf("a partner account rendered %v", results[0].IsPartner)
	}
	if results[1].IsPartner == nil || *results[1].IsPartner {
		t.Errorf("a checked non-partner rendered %v, want a literal false", results[1].IsPartner)
	}
}
