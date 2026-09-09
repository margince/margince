// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A seat holding no organization grant at all. VisibleSubset consults the
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

func offerNamingBuyer(org ids.UUID) crmcontracts.Offer {
	buyer := openapi_types.UUID(org)
	snapshot := map[string]interface{}{"display_name": "Meridian Labs"}
	return crmcontracts.Offer{BuyerOrgId: &buyer, BuyerSnapshot: &snapshot}
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
	if offer.BuyerOrgId != nil {
		t.Errorf("the caller's own offer still names buyer organization %v", *offer.BuyerOrgId)
	}
	if offer.BuyerSnapshot != nil {
		t.Errorf("the caller's own offer still carries the buyer snapshot %v — the frozen block names the company, which is strictly more than the id withheld beside it", *offer.BuyerSnapshot)
	}
}

// Both fields or neither. Withholding the id and leaving the snapshot hands
// back the NAME of an organization whose id was judged too much to disclose,
// which is the worse half of the pair rather than a partial fix.
func TestWithholdingTakesTheSnapshotWithTheReference(t *testing.T) {
	offers := []crmcontracts.Offer{offerNamingBuyer(ids.NewV7()), offerNamingBuyer(ids.NewV7())}
	if err := withholdUnreadableBuyer(deskWithoutCompanyAccess(), nil, offers); err != nil {
		t.Fatalf("withholding across the page: %v", err)
	}
	for i, o := range offers {
		if o.BuyerOrgId != nil || o.BuyerSnapshot != nil {
			t.Errorf("offer %d still names its buyer (id=%v snapshot=%v)", i, o.BuyerOrgId, o.BuyerSnapshot)
		}
	}
}

// An offer with no buyer names nothing to probe, so the page costs nothing and
// the fields stay as they were rather than being rewritten to the same value.
func TestAnOfferWithNoBuyerIsLeftAlone(t *testing.T) {
	offer := crmcontracts.Offer{}
	if err := withholdUnreadableBuyerOn(deskWithoutCompanyAccess(), nil, &offer); err != nil {
		t.Fatalf("withholding on an offer with no buyer: %v", err)
	}
	if offer.BuyerOrgId != nil || offer.BuyerSnapshot != nil {
		t.Errorf("an offer with no buyer gained one: id=%v snapshot=%v", offer.BuyerOrgId, offer.BuyerSnapshot)
	}
}
