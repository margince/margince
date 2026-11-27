// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetChannelDeliveryReview exposes delivery repair through the existing record authority.
func (h Handlers) GetChannelDeliveryReview(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	review, err := h.store.DeliveryReview(r.Context(), string(kind), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireDeliveryReview(review))
}

// ReviewChannelDeliveries applies explicit recovery choices through record management authority.
func (h Handlers) ReviewChannelDeliveries(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	var in crmcontracts.ReviewChannelDeliveryRequest
	if !httperr.Decode(w, r, &in) {
		return
	}
	input, err := deliveryReviewInput(in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	review, err := h.store.ReviewDeliveries(r.Context(), string(kind), ids.UUID(id), input)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireDeliveryReview(review))
}

func wireDeliveryReview(review DeliveryReview) crmcontracts.ChannelDeliveryReview {
	out := crmcontracts.ChannelDeliveryReview{LinkId: openapi_types.UUID(review.Link), OverviewCardPaused: review.CardPaused, Deliveries: make([]crmcontracts.ChannelUncertainDelivery, 0, len(review.Deliveries))}
	out.ProviderCopies = &crmcontracts.ChannelProviderCopies{RetainedMessages: review.Copies.RetainedMessages, ConfirmedRedactions: review.Copies.ConfirmedRedactions, UnverifiedWithdrawals: review.Copies.UnverifiedWithdrawals, UnconfirmedDeliveries: review.Copies.UnconfirmedDeliveries}
	for _, item := range review.Deliveries {
		out.Deliveries = append(out.Deliveries, crmcontracts.ChannelUncertainDelivery{Id: openapi_types.UUID(item.ID), State: crmcontracts.ChannelUncertainDeliveryState(item.State), CreatedAt: item.Created, UpdateCount: item.Updates, OverviewCard: item.Card})
	}
	return out
}
