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
	"context"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/commsauthzwire"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetCommunicationReview serves GET /communication-reviews/{id}.
//
// Wire-only. The store owns who may read which review, and it must: a handler
// that decided scope here would be a second answer to a question the store
// already answers, and the two would disagree the first time either changed.
func (h Handlers) GetCommunicationReview(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	review, err := h.store.ServedReview(r.Context(), ids.UUID(id))
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, review)
}

// ServedReview is the review as GET /communication-reviews/{id} serves it to
// this caller. A refusal that embeds its review reads it here, so the two
// answers share one scope, one redaction and one set of actions.
func (s *Store) ServedReview(ctx context.Context, id ids.UUID) (crmcontracts.CommunicationReview, error) {
	review, err := s.ReviewForReader(ctx, id)
	if err != nil {
		return crmcontracts.CommunicationReview{}, err
	}
	return wireReview(review), nil
}

// ServedReview is Store.ServedReview, for the send path that holds the gate.
func (g *Gate) ServedReview(ctx context.Context, id ids.UUID) (crmcontracts.CommunicationReview, error) {
	return g.store.ServedReview(ctx, id)
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
		refusals = append(refusals, wireRefusal(refusal))
	}
	revision := review.AssessmentRevision()
	out := crmcontracts.CommunicationReview{
		AssessmentRevision: &revision,
		Id:                 openapi_types.UUID(review.ID),
		State:              crmcontracts.CommunicationReviewState(review.State),
		Kind:               crmcontracts.CommunicationReviewKind(review.Kind),
		ReasonCode:         review.ReasonCode,
		Refusals:           refusals,
	}
	// Null rather than a zero uuid when nothing was held. A caller reading
	// 00000000-… would have an id it could take to a lookup that answers not
	// found, which reads as a broken reference rather than as an absent one.
	if !review.IntentID.IsZero() {
		intent := openapi_types.UUID(review.IntentID)
		out.DeliveryIntentId = &intent
	}
	// SERVED ON EVERY REVIEW, not only on ones somebody may direct. Whether
	// this reader holds the grant is a question the direct-send door answers,
	// and withholding the words here would make a surface guess whether to ask
	// for them — one more round trip, and a modal that opens empty.
	warning := TheOverrideWarning()
	out.Warning = &crmcontracts.OverrideWarning{
		Version: warning.Version, Text: warning.Text,
	}
	warnings := make([]crmcontracts.OverrideWarning, 0, len(review.Warnings))
	for _, served := range review.Warnings {
		warnings = append(warnings, crmcontracts.OverrideWarning{Version: served.Version, Text: served.Text})
	}
	out.Warnings = &warnings
	if correction := review.Correction; correction != nil {
		out.WordingCorrection = &crmcontracts.WordingCorrectionContext{
			InstructionId: openapi_types.UUID(correction.InstructionID), Version: correction.Version,
			Subject: correction.Subject, Body: correction.Body, HtmlBody: correction.HTMLBody,
			WordingDigest: correction.WordingDigest, ReasonCode: correction.ReasonCode,
			Explanation: correction.Explanation, WarningVersion: correction.WarningVersion,
		}
	}
	return out
}

// wireRefusal renders one refused recipient as the rep may read it.
func wireRefusal(refusal RefusedRecipient) crmcontracts.RefusedRecipient {
	wire := crmcontracts.RefusedRecipient{
		Address:    refusal.Address,
		ReasonCode: refusal.ReasonCode,
	}
	if len(refusal.FooterPreview) > 0 {
		wire.FooterPreview = &refusal.FooterPreview
	}
	if len(refusal.FooterFindings) > 0 {
		wire.FooterFindings = commsauthzwire.FooterFindings(refusal.FooterFindings)
	}
	wire.Provision = commsauthzwire.Provision(refusal.Provision)
	if refusal.SendingEntity != nil {
		id := openapi_types.UUID(*refusal.SendingEntity)
		wire.SendingEntityId = &id
	}
	if refusal.SendingEntityName != "" {
		name := refusal.SendingEntityName
		wire.SendingEntityName = &name
	}
	if refusal.Requested != "" {
		requested := refusal.Requested
		wire.RequestedCategory = &requested
	}
	if refusal.Category != "" {
		category := refusal.Category
		wire.Category = &category
	}
	if refusal.Country != "" {
		country := refusal.Country
		wire.Country = &country
	}
	if refusal.CountrySource != "" {
		source := crmcontracts.RefusedRecipientCountrySource(refusal.CountrySource)
		wire.CountrySource = &source
	}
	if refusal.SubjectKind != "" {
		kind := crmcontracts.RefusedRecipientSubjectKind(refusal.SubjectKind)
		wire.SubjectKind = &kind
	}
	if subject, err := ids.Parse(refusal.SubjectID); err == nil {
		id := openapi_types.UUID(subject)
		wire.SubjectId = &id
	}
	return wire
}

// RecordCommunicationContext takes one rep's statement about one contact and
// answers with the review as it now stands.
//
// Thin transport, like its siblings. Which refusals a statement can answer, who
// may make one and what it is written to are the store's.
//
// THE RESPONSE IS THE RE-READ REVIEW rather than a 204, and that is the fix for
// the defect this endpoint shipped with the first time: a caller has to be able
// to see that something changed. A review that still names the same refusal
// after a statement was accepted is a rep about to type the sentence again.
func (h Handlers) RecordCommunicationContext(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id,
) {
	var body crmcontracts.RecordCommunicationContextRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	review, err := h.store.RecordContext(r.Context(), ids.UUID(id), RecordContextInput{
		SubjectID:  ids.From[ids.ContactKind](ids.UUID(body.SubjectId)),
		Kind:       string(body.Kind),
		Note:       body.Note,
		OccurredAt: body.OccurredAt,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireReview(review))
}
