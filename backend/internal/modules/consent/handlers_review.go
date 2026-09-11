// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Opening a review the refusal named.
//
// A refused send answers a reference and, until this existed, nothing could
// resolve it: the row was written, the rep was handed its id, and there was no
// door. They had a reference to a record they could not read, which is barely
// better than the bare error code it replaced.

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetCommunicationReview serves GET /communication-reviews/{id}.
//
// Wire-only. The store owns who may read which review, and it must: a handler
// that decided scope here would be a second answer to a question the store
// already answers, and the two would disagree the first time either changed.
func (h Handlers) GetCommunicationReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	review, err := h.store.ReviewForReader(r.Context(), ids.UUID(id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireReview(review))
}

// wireReview renders a review for the rep who asked for it.
//
// The refusals are rendered as an EMPTY LIST rather than omitted when there are
// none. A review whose subject has since been erased holds no refusals, and a
// client distinguishing "absent" from "empty" would be reading a difference
// that means nothing — both say this review no longer names anybody.
func wireReview(review Review) crmcontracts.CommunicationReview {
	refusals := make([]crmcontracts.RefusedRecipient, 0, len(review.Refusals))
	for _, refusal := range review.Refusals {
		wire := crmcontracts.RefusedRecipient{
			Address:    refusal.Address,
			ReasonCode: refusal.ReasonCode,
		}
		if refusal.Category != "" {
			category := refusal.Category
			wire.Category = &category
		}
		if refusal.SubjectKind != "" {
			kind := crmcontracts.RefusedRecipientSubjectKind(refusal.SubjectKind)
			wire.SubjectKind = &kind
		}
		if subject, err := ids.Parse(refusal.SubjectID); err == nil {
			id := openapi_types.UUID(subject)
			wire.SubjectId = &id
		}
		refusals = append(refusals, wire)
	}
	out := crmcontracts.CommunicationReview{
		Id:         openapi_types.UUID(review.ID),
		State:      crmcontracts.CommunicationReviewState(review.State),
		Kind:       crmcontracts.CommunicationReviewKind(review.Kind),
		ReasonCode: review.ReasonCode,
		Refusals:   refusals,
	}
	// Null rather than a zero uuid when nothing was held. A caller reading
	// 00000000-… would have an id it could take to a lookup that answers not
	// found, which reads as a broken reference rather than as an absent one.
	if !review.IntentID.IsZero() {
		intent := openapi_types.UUID(review.IntentID)
		out.DeliveryIntentId = &intent
	}
	// Null rather than a zero uuid for the same reason the intent is: an id a
	// caller could take to a lookup that answers not found reads as a broken
	// reference rather than an absent one.
	if !review.ApprovalID.IsZero() {
		card := openapi_types.UUID(review.ApprovalID)
		out.ApprovalId = &card
	}
	return out
}

// RequestCommunicationDecision hands a refused send to somebody who may direct
// it.
//
// Thin transport. Who may ask, what state a review must be in and what the card
// carries are all the store's, and the approvals engine decides who may answer.
func (h Handlers) RequestCommunicationDecision(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
) {
	var body crmcontracts.RequestCommunicationDecisionRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	note := ""
	if body.Note != nil {
		note = *body.Note
	}
	approvalID, err := h.store.RequestDecision(r.Context(), ids.UUID(id), note)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, crmcontracts.CommunicationDecisionRequested{
		ApprovalId: openapi_types.UUID(approvalID),
	})
}

// ListCommunicationReviews serves GET /communication-reviews.
//
// Wire-only, for GetCommunicationReview's reason: the store owns who may read
// this queue, and a handler deciding it here would be a second answer to a
// question the store already answers.
func (h Handlers) ListCommunicationReviews(
	w http.ResponseWriter, r *http.Request, params crmcontracts.ListCommunicationReviewsParams,
) {
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	reviews, total, err := h.store.AwaitingDecision(r.Context(), limit)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	out := make([]crmcontracts.CommunicationReview, 0, len(reviews))
	for _, review := range reviews {
		out = append(out, wireReview(review))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.CommunicationReviewList{
		Data: out, Total: total,
	})
}
