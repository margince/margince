// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A seat holding no company grant at all. VisibleSubset consults the
// object grant before any row scope and answers an empty set for this caller
// without issuing a statement, which is why these cases need no transaction —
// the nil tx is the assertion that none is reached.
func deskWithoutCompanyAccess() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				"deal":  {Read: true, Update: true},
				"offer": {Read: true, Update: true},
			},
		},
	})
}

func offerNamingBuyer(company ids.UUID) crmcontracts.Offer {
	buyer := openapi_types.UUID(company)
	snapshot := map[string]any{"display_name": "Meridian Labs"}
	rendering := "offers/meridian-labs.pdf"
	return crmcontracts.Offer{BuyerCompanyId: &buyer, BuyerSnapshot: &snapshot, PdfAssetRef: &rendering}
}

// The read-back is the whole reason this spelling exists. Withholding against
// a slice literal mutates a COPY, so a caller that forgets to read the element
// back keeps the reference — and nothing fails, because the offer still reads
// correctly and the buyer is still on it.
func TestWithholdingABuyerReachesTheCallersOwnOffer(t *testing.T) {
	offer := offerNamingBuyer(ids.NewV7())
	if err := withholdUnreadableBuyerOn(deskWithoutCompanyAccess(), nil, &offer); err != nil {
		t.Fatalf("withholding the buyer: %v", err)
	}
	if offer.BuyerCompanyId != nil {
		t.Errorf("the caller's own offer still names buyer company %v", *offer.BuyerCompanyId)
	}
	if offer.BuyerSnapshot != nil {
		t.Errorf("the caller's own offer still carries the buyer snapshot %v — the frozen block names the company, which is strictly more than the id withheld beside it", *offer.BuyerSnapshot)
	}
}

// Both fields or neither. Withholding the id and leaving the snapshot hands
// back the NAME of a company whose id was judged too much to disclose,
// which is the worse half of the pair rather than a partial fix.
func TestWithholdingTakesTheSnapshotWithTheReference(t *testing.T) {
	offers := []crmcontracts.Offer{offerNamingBuyer(ids.NewV7()), offerNamingBuyer(ids.NewV7())}
	if err := withholdUnreadableBuyer(deskWithoutCompanyAccess(), nil, offers); err != nil {
		t.Fatalf("withholding across the page: %v", err)
	}
	for i, o := range offers {
		if o.BuyerCompanyId != nil || o.BuyerSnapshot != nil {
			t.Errorf("offer %d still names its buyer (id=%v snapshot=%v)", i, o.BuyerCompanyId, o.BuyerSnapshot)
		}
	}
}

// The rendering goes with the buyer it prints. The stored PDF carries the legal
// block as its renderer could read it, so a reader denied the buyer is not
// handed the document that names it.
func TestWithholdingTakesTheRenderingWithTheBuyer(t *testing.T) {
	offers := []crmcontracts.Offer{offerNamingBuyer(ids.NewV7())}
	if err := withholdUnreadableBuyer(deskWithoutCompanyAccess(), nil, offers); err != nil {
		t.Fatalf("withholding the buyer: %v", err)
	}
	if offers[0].PdfAssetRef != nil {
		t.Errorf("the offer still names its rendering %q, which prints the buyer withheld beside it", *offers[0].PdfAssetRef)
	}
}

// A new buyer retires the rendering, because the read judges the stored PDF by
// the buyer the offer names, and that PDF still prints the old one.
func TestANewBuyerRetiresTheRendering(t *testing.T) {
	offer := offerNamingBuyer(ids.NewV7())
	p := storekit.NewPatch()
	retired := retireRenderingOnBuyerChange(p, offer, ids.From[ids.CompanyKind](ids.NewV7()))
	if cleared, set := p.After()["pdf_asset_ref"]; !set || cleared != nil {
		t.Errorf("a new buyer left the rendering in place (set=%v, value=%v)", set, cleared)
	}
	// The ref is handed back so the caller reclaims the object nothing names now.
	if retired == nil || *retired != *offer.PdfAssetRef {
		t.Errorf("the retirement answered %v, want the ref it cleared, %q", retired, *offer.PdfAssetRef)
	}
}

// An offer never rendered has nothing to retire, so a buyer change on it writes no
// rendering column at all.
func TestABuyerChangeOnAnUnrenderedOfferRetiresNothing(t *testing.T) {
	offer := offerNamingBuyer(ids.NewV7())
	offer.PdfAssetRef = nil
	p := storekit.NewPatch()
	if retired := retireRenderingOnBuyerChange(p, offer, ids.From[ids.CompanyKind](ids.NewV7())); retired != nil || !p.Empty() {
		t.Errorf("an unrendered offer retired %v: %v", retired, p.After())
	}
}

// The header patch carries exactly the fields the edit names, each beside the
// value it replaces, which is what lets the audit row say what the edit changed.
func TestTheHeaderPatchCarriesEachNamedField(t *testing.T) {
	intro, terms := "Was intro", "Was terms"
	until := openapi_types.Date{Time: time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)}
	current := crmcontracts.Offer{Currency: "EUR", ValidUntil: &until, IntroText: &intro, TermsText: &terms}
	newCurrency, newUntil, newIntro, newTerms := "USD", "2026-12-31", "New intro", "New terms"
	p, retired, err := offerHeaderPatch(context.Background(), nil, current, UpdateOfferInput{
		Currency: &newCurrency, ValidUntil: &newUntil, IntroText: &newIntro, TermsText: &newTerms,
	})
	if err != nil || retired != nil {
		t.Fatalf("patching the plain header fields answered retired=%v err=%v", retired, err)
	}
	// The before-image is the current row's own field, compared by identity.
	for _, field := range []struct {
		column        string
		before, after any
	}{
		{"currency", "EUR", newCurrency},
		{"valid_until", &until, newUntil},
		{"intro_text", &intro, newIntro},
		{"terms_text", &terms, newTerms},
	} {
		if p.Before()[field.column] != field.before {
			t.Errorf("the patch records %s as changing from something other than the current row's own value", field.column)
		}
		if got := p.After()[field.column]; got != field.after {
			t.Errorf("the patch sets %s to %v, want %v", field.column, got, field.after)
		}
	}
	if _, named := p.After()["buyer_company_id"]; named {
		t.Error("the patch names a buyer the edit never sent")
	}
}

// The same buyer saved again keeps it. A form resaves the fields nobody touched,
// and a PDF that vanished on every save of the header would be a broken feature.
func TestTheSameBuyerKeepsTheRendering(t *testing.T) {
	company := ids.NewV7()
	p := storekit.NewPatch()
	if retired := retireRenderingOnBuyerChange(p, offerNamingBuyer(company), ids.From[ids.CompanyKind](company)); retired != nil || !p.Empty() {
		t.Errorf("saving the same buyer again retired the rendering %v: %v", retired, p.After())
	}
}

// An offer with no buyer names nothing to probe, so the page costs nothing and
// the fields stay as they were rather than being rewritten to the same value.
func TestAnOfferWithNoBuyerIsLeftAlone(t *testing.T) {
	rendering := "offers/no-buyer.pdf"
	offer := crmcontracts.Offer{PdfAssetRef: &rendering}
	if err := withholdUnreadableBuyerOn(deskWithoutCompanyAccess(), nil, &offer); err != nil {
		t.Fatalf("withholding on an offer with no buyer: %v", err)
	}
	if offer.BuyerCompanyId != nil || offer.BuyerSnapshot != nil {
		t.Errorf("an offer with no buyer gained one: id=%v snapshot=%v", offer.BuyerCompanyId, offer.BuyerSnapshot)
	}
	if offer.PdfAssetRef == nil {
		t.Error("an offer with no buyer lost its rendering — a document that names nobody has nothing to withhold")
	}
}

// No principal, no verdict — and the offer does not travel.
//
// The withholding decides visibility against the caller; with no actor bound
// there is no caller to decide about, and the honest answer is an error rather
// than an offer whose buyer was never judged. Returning the offer unchanged
// would be the worst reading of "could not tell": the reference reaches whoever
// asked, on a path that failed to ask the question.
func TestWithholdingFailsRatherThanPassingAnUnjudgedBuyerThrough(t *testing.T) {
	company := ids.NewV7()
	offer := offerNamingBuyer(company)
	err := withholdUnreadableBuyerOn(context.Background(), nil, &offer)
	if err == nil {
		t.Fatal("withholding answered nil with no actor bound — the buyer travelled without anyone deciding it could")
	}
	if offer.BuyerCompanyId == nil || ids.UUID(*offer.BuyerCompanyId) != company {
		t.Errorf("the offer was rewritten on a failed verdict (id=%v) — a caller that ignores the error would then read a withholding that never happened", offer.BuyerCompanyId)
	}
}
