// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The no-login confirm-your-details transport. The public middleware has
// already turned an unknown token away and bound the workspace plus a system
// principal; each handler resolves the token again for the contact it names —
// the same infra read the preference surface makes — and then drives the store.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// GetConfirmDetails implements (GET /public/confirm/{token}): one contact's own
// view of what is held about them.
//
// Resolving records a FETCH always and an OPENING only when the request does
// not announce itself as a machine. The opening is the middle of the
// ask-to-click chain the proof row later refers to, and a scanner prefetching
// the link must not write it — see linkfetch.go.
//
// The request is read HERE because this is where one exists. The store is given
// the answer rather than the headers, so the rule lives in one place and every
// other surface that ever resolves a token has to state which it is.
func (h Handlers) GetConfirmDetails(w http.ResponseWriter, r *http.Request, token string) {
	ref, err := h.store.ResolveConfirmToken(r.Context(), token, WhatFetchedThis(r))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	// A consent link is answered with the subscription question and nothing
	// else. Its mail said "confirm this subscription"; serving the record card
	// here would hand whoever holds the link the contact's name, employer,
	// address, phone and the whole provenance trail — wider than the mail
	// described, and wider than the write side of this same link allows.
	//
	// The read is the disclosure, so it carries the same gate the submit does.
	if ref.Kind == LinkConsentConfirmation {
		card, err := h.store.consentCardFor(r.Context(), ref)
		if err != nil {
			writeConsentErr(w, r, err)
			return
		}
		httperr.WriteJSON(w, http.StatusOK, wireSubscriptionCard(card))
		return
	}
	// EVERY OTHER KIND IS REFUSED, rather than falling through to the record.
	//
	// The record card is the widest thing this endpoint can disclose — name,
	// employer, address, phone, provenance — so it must be reached by a kind
	// that asked for it, never by not matching the other arm. A link kind added
	// to the table tomorrow would otherwise serve the record to whoever holds
	// it, silently, and the handler that needed updating would look untouched.
	if ref.Kind != LinkRecordConfirmation {
		writeConsentErr(w, r, apperrors.ErrNotFound)
		return
	}
	card, err := h.store.confirmCardFor(r.Context(), ref.ContactID)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireConfirmCard(card))
}

// SubmitConfirmDetails implements (POST /public/confirm/{token}): the answer
// coming back. The store spends the link and records everything in one
// transaction, so a replayed submit refuses rather than writing twice.
func (h Handlers) SubmitConfirmDetails(w http.ResponseWriter, r *http.Request, token string) {
	var req crmcontracts.SubmitConfirmDetailsJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	receipts, err := h.store.SubmitConfirmation(r.Context(), token, submissionFromWire(req))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireRightsCaseReceipts(receipts))
}

// wireRightsCaseReceipts renders the receipts for the page that has to show
// them. An EMPTY LIST rather than a null: a submission carrying only a
// marketing answer opens no case, and a page distinguishing "no cases" from
// "the field was absent" would be reading a difference that means nothing.
func wireRightsCaseReceipts(receipts []RightsCaseReceipt) crmcontracts.ConfirmSubmissionReceipt {
	out := crmcontracts.ConfirmSubmissionReceipt{
		Cases: make([]crmcontracts.RightsCaseReceipt, 0, len(receipts)),
	}
	for _, receipt := range receipts {
		wire := crmcontracts.RightsCaseReceipt{
			Kind:      crmcontracts.RightsCaseReceiptKind(receipt.Kind),
			Reference: receipt.Reference,
		}
		if receipt.Field != "" {
			field := receipt.Field
			wire.Field = &field
		}
		out.Cases = append(out.Cases, wire)
	}
	return out
}

// submissionFromWire reads the request into the store's shape. A repeated field
// keeps the LAST value rather than refusing: the page sends at most one box per
// field, so a duplicate is a client bug, and taking the last is what a form post
// would do anyway.
func submissionFromWire(req crmcontracts.SubmitConfirmDetailsJSONRequestBody) ConfirmSubmission {
	in := ConfirmSubmission{Corrections: map[string]string{}}
	if req.Corrections != nil {
		for _, c := range *req.Corrections {
			in.Corrections[string(c.Field)] = c.Value
		}
	}
	if req.RequestErasure != nil {
		in.RequestErasure = *req.RequestErasure
	}
	if req.MarketingChoice != nil {
		in.MarketingChoice = string(*req.MarketingChoice)
	}
	if req.MarketingWording != nil {
		in.MarketingWording = *req.MarketingWording
	}
	return in
}

// wireConfirmCard renders the card. marketing_state is spelled 'unknown' rather
// than empty, matching the preference surface's own vocabulary: no record and a
// withdrawal are different answers, and the page shows them differently.
func wireConfirmCard(card ConfirmCard) crmcontracts.RecordConfirmationPage {
	state := card.Marketing
	if state == "" {
		state = "unknown"
	}
	origins := make([]crmcontracts.ConfirmFieldOrigin, 0, len(card.Provenance))
	for _, o := range card.Provenance {
		origins = append(origins, crmcontracts.ConfirmFieldOrigin{
			Field: o.Field, RecordedAt: o.RecordedAt, Source: o.Source,
		})
	}
	return crmcontracts.RecordConfirmationPage{
		// The discriminator, and the reason this endpoint has an honest schema
		// again. Both bodies used to arrive carrying nothing that said which
		// they were, so a client had to guess from which fields happened to be
		// present — and the published schema described only this one.
		Kind:           crmcontracts.RecordConfirmationPageKindRecordConfirmation,
		FullName:       card.FullName,
		Title:          card.Title,
		Company:        card.Company,
		Email:          card.Email,
		Phone:          card.Phone,
		MarketingState: crmcontracts.RecordConfirmationPageMarketingState(state),
		Provenance:     origins,
	}
}

// wireSubscriptionCard is the other branch, and it exists so the subscription
// answer carries its discriminator too.
//
// Before this the handler wrote consent.SubscriptionCard — a module type with
// its own json tags — straight to the response. It serialized acceptably, which
// is exactly why nothing caught that the contract did not describe it: the
// generated client typed every 200 from this endpoint as the record card, so
// the subscription page read `provenance` off a body with three fields.
func wireSubscriptionCard(card SubscriptionCard) crmcontracts.SubscriptionConfirmationPage {
	state := card.State
	if state == "" {
		state = "unknown"
	}
	return crmcontracts.SubscriptionConfirmationPage{
		Kind:         crmcontracts.SubscriptionConfirmationPageKindSubscriptionConfirmation,
		PurposeKey:   card.PurposeKey,
		PurposeLabel: card.PurposeLabel,
		State:        crmcontracts.SubscriptionConfirmationPageState(state),
	}
}
