// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The six bespoke resolvers' own answers (commandnested.go): five stand
// down the same way the six record-seam-unserved archivable types do
// (list, tag, offer), and one — createOffer's parent deal — refuses the same
// two ways patchResolver's own target does.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// addListMember, applyTag and the three offer-line-item commands all stage
// OUTSIDE the record seam (list, tag and offer are none of them served) —
// the id alone names the target, the same shape
// TestCustomFieldCommandsStageAndAdmitOutsideTheRecordSeam proves for
// retire/update-options. Guards is asked — StageSubject (command.go) runs it
// before Subject, which is what makes this an ADMIT assertion and not only a
// staging one — against a provider that fails EVERY read, so a resolver that
// consulted the seam anyway fails here rather than passing on a lenient stub.
func TestListTagAndLineItemCommandsStageAndAdmitOutsideTheRecordSeam(t *testing.T) {
	id, lineItemID := ids.NewV7(), ids.NewV7()
	cases := []struct {
		name           string
		call           GovernedCall
		wantTargetType string
	}{
		{"apply_tag", NewApplyTagCall(unreadableProvider{}, ApplyTagCommand{ID: id}), "tag"},
		{"add_offer_line_item", NewAddOfferLineItemCall(unreadableProvider{}, AddOfferLineItemCommand{ID: id}), "offer"},
		{
			"update_offer_line_item",
			NewUpdateOfferLineItemCall(unreadableProvider{}, UpdateOfferLineItemCommand{ID: id, LineItemID: lineItemID}),
			"offer",
		},
		{
			"remove_offer_line_item",
			NewRemoveOfferLineItemCall(unreadableProvider{}, RemoveOfferLineItemCommand{ID: id, LineItemID: lineItemID}),
			"offer",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, err := StageSubject(context.Background(), c.call)
			if err != nil {
				t.Fatalf("staging outside the record seam answered %v, want it staged", err)
			}
			if info.TargetType != c.wantTargetType || info.TargetID != id {
				t.Errorf("staged target = (%s,%s), want (%s,%s)", info.TargetType, info.TargetID, c.wantTargetType, id)
			}
		})
	}
}

// The two offer-line-item commands with a second path operand name the LINE
// ITEM in the summary, distinct per line the same way removeStakeholder's
// own summary names PersonID.
func TestOfferLineItemSummariesNameTheLineItem(t *testing.T) {
	offerID, lineItemID := ids.NewV7(), ids.NewV7()

	updateInfo, err := NewUpdateOfferLineItemCall(unreadableProvider{}, UpdateOfferLineItemCommand{ID: offerID, LineItemID: lineItemID}).
		Subject(context.Background())
	if err != nil {
		t.Fatalf("naming the update subject answered %v, want no error", err)
	}
	if !strings.Contains(updateInfo.Summary, lineItemID.String()) {
		t.Errorf("update summary %q does not name the line item", updateInfo.Summary)
	}

	removeInfo, err := NewRemoveOfferLineItemCall(unreadableProvider{}, RemoveOfferLineItemCommand{ID: offerID, LineItemID: lineItemID}).
		Subject(context.Background())
	if err != nil {
		t.Fatalf("naming the remove subject answered %v, want no error", err)
	}
	if !strings.Contains(removeInfo.Summary, lineItemID.String()) {
		t.Errorf("remove summary %q does not name the line item", removeInfo.Summary)
	}
}

// margince/margince#1046: createOffer's staged target must carry NO
// id. The routed {id} on POST /v1/deals/{id}/offers is the DEAL the offer
// nests under, not an offer id — the offer does not exist yet — so pairing
// target_entity_type=offer with that id would name a target that resolves to
// no row, or to an unrelated offer that happens to share the id space. A
// resolver of its own is the fix; this is the assertion that pins it.
func TestCreateOfferStagesNoID(t *testing.T) {
	dealID := ids.NewV7()
	provider := stubRecordProvider{rec: stagedRecord(datasource.EntityDeal, dealID, true)}
	call := NewCreateOfferCall(provider, CreateOfferCommand{DealID: dealID, Fields: json.RawMessage(`{"currency":"EUR"}`)})

	info, err := StageSubject(context.Background(), call)
	if err != nil {
		t.Fatalf("staging a nested offer create answered %v, want it staged", err)
	}
	if info.TargetType != "offer" {
		t.Errorf("staged target_type = %q, want \"offer\"", info.TargetType)
	}
	if !info.TargetID.IsZero() {
		t.Errorf("staged target_id = %s, want zero — the offer does not exist yet, and the routed id names "+
			"the DEAL, not an offer (margince/margince#1046)", info.TargetID)
	}
	if !strings.Contains(info.Summary, dealID.String()) {
		t.Errorf("summary %q does not name the parent deal", info.Summary)
	}
}

// createOffer's Guards reads the DEAL, not an offer — deal IS served by the
// record seam, so this is a real read, unlike the five stand-downs above.
func TestCreateOfferGuardsRefuseAnUnreadableDeal(t *testing.T) {
	call := NewCreateOfferCall(unreadableProvider{}, CreateOfferCommand{DealID: ids.NewV7(), Fields: json.RawMessage(`{}`)})

	if err := call.Guards(context.Background()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("guarding an unreadable deal answered %v, want the row-scope miss", err)
	}
}

// A deal held in another system of record is refused too — an approval for
// it could never be released, the offer's create included.
func TestCreateOfferGuardsRefuseADealHeldElsewhere(t *testing.T) {
	call := NewCreateOfferCall(elsewhereProvider{}, CreateOfferCommand{DealID: ids.NewV7(), Fields: json.RawMessage(`{}`)})

	if err := call.Guards(context.Background()); !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Fatalf("guarding a mirrored deal answered %v, want the unsupported-by-SoR refusal", err)
	}
}

// A served, readable deal is admitted rather than refused.
func TestCreateOfferGuardsAdmitAReadableDeal(t *testing.T) {
	id := ids.NewV7()
	provider := stubRecordProvider{rec: stagedRecord(datasource.EntityDeal, id, true)}
	if err := NewCreateOfferCall(provider, CreateOfferCommand{DealID: id, Fields: json.RawMessage(`{}`)}).
		Guards(context.Background()); err != nil {
		t.Fatalf("guarding a readable, authoritative deal answered %v, want it admitted", err)
	}
}
