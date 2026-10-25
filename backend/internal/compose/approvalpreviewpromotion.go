// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func promotionApprovalInput(approval crmcontracts.Approval) (contacts.PromoteLeadInput, error) {
	if approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeLead) || approval.TargetEntityId == nil || approval.TargetVersion == nil || approval.ProposedChange == nil {
		return contacts.PromoteLeadInput{}, apperrors.ErrInvalidArgument
	}
	var in contacts.PromoteLeadInput
	if _, rest := (*approval.ProposedChange)["operation"]; rest {
		req, err := commercialRequest[crmcontracts.PromoteLeadRequest](approval)
		if err != nil {
			return in, err
		}
		in = contacts.PromotionRequestInput(req)
	} else {
		req, err := commercialRequest[struct {
			LeadID       ids.UUID `json:"lead_id"`
			Trigger      string   `json:"trigger"`
			EvidenceNote *string  `json:"evidence_note"`
		}](approval)
		if err != nil {
			return in, err
		}
		if req.LeadID != ids.UUID(*approval.TargetEntityId) {
			return in, apperrors.ErrInvalidArgument
		}
		in.Trigger, in.EvidenceNote = req.Trigger, req.EvidenceNote
	}
	in.IfVersion = new(int64(*approval.TargetVersion))
	return in, nil
}

func (r approvalPreviewReader) promotionPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	in, err := promotionApprovalInput(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	consents := consent.NewStore(r.db)
	store := contacts.NewStore(r.db).WithStopCarrier(consents).WithSatelliteCarriers(consents, nil).WithDealOpener(leadDealOpener{deals: r.deals})
	plan, err := store.PreviewLeadContactChange(ctx, ids.From[ids.LeadKind](ids.UUID(*approval.TargetEntityId)), in)
	if errors.Is(err, apperrors.ErrVersionSkew) {
		out.State = previewStaleState
		return out, nil
	}
	if previewUnavailable(err) || errors.Is(err, apperrors.ErrConflict) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.CurrentVersion = new(int(plan.LeadVersion))
	details, err := r.promotionDetails(ctx, in, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details = &details
	effects := []string{"promote_lead_archive_source"}
	if plan.Create != nil {
		effects = append(effects, "promote_create_contact")
	} else {
		effects = append(effects, "promote_fill_contact")
	}
	if len(plan.ConsentLinks.WithdrawalCredentials)+len(plan.ConsentLinks.CommunicationBases) > 0 {
		effects = append(effects, "promote_move_consent_links")
	}
	if plan.Deal != nil {
		effects = append(effects, "promote_open_deal_seat_contact")
	}
	if plan.ReviewContact != nil {
		effects = append(effects, "promote_queue_duplicate_review")
	}
	out.Effects, out.State = &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) promotionDetails(ctx context.Context, in contacts.PromoteLeadInput, plan contacts.LeadContactChange) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("promotion_trigger", in.Trigger)}
	if in.EvidenceNote != nil {
		details = append(details, previewDetail("promotion_note", *in.EvidenceNote))
	}
	if in.EvidenceActivityID != nil {
		value, err := r.referenceValue(ctx, "activity_id", in.EvidenceActivityID.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_evidence", Value: value})
	}
	contact, err := r.promotionContactDetails(ctx, plan)
	if err != nil {
		return nil, err
	}
	details = append(details, contact...)
	carried, err := r.promotionCarryDetails(ctx, plan.LeadPromotionCarry, false)
	if err != nil {
		return nil, err
	}
	details = append(details, carried...)
	if plan.Deal != nil {
		deal, err := r.promotionDealDetails(ctx, *plan.Deal)
		if err != nil {
			return nil, err
		}
		details = append(details, deal...)
	}
	return details, nil
}

func (r approvalPreviewReader) promotionContactDetails(ctx context.Context, plan contacts.LeadContactChange) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var details []crmcontracts.ApprovalPreviewDetail
	if plan.Create != nil {
		var err error
		details, err = r.promotionNewContactDetails(ctx, plan)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		details, err = r.promotionExistingContactDetails(ctx, plan)
		if err != nil {
			return nil, err
		}
	}
	if plan.ReviewContact != nil {
		candidate, err := r.referenceValue(ctx, companionContactField, plan.ReviewContact.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_duplicate_candidate", Value: candidate})
	}
	return details, nil
}

func (r approvalPreviewReader) promotionCarryDetails(ctx context.Context, plan contacts.LeadPromotionCarry, sourceOnly bool) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, activity := range plan.Activities {
		value, err := r.referenceValue(ctx, "activity_id", activity.ID.String())
		if err != nil {
			return nil, err
		}
		field := "promotion_move_activity"
		if activity.AlreadyOnContact {
			field = "promotion_remove_duplicate_activity_link"
		}
		if sourceOnly {
			field = "creation_source_activity"
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: value})
	}
	if plan.WithheldActivities > 0 {
		field := "promotion_move_withheld_activities"
		if sourceOnly {
			field = "creation_source_withheld_activities"
		}
		details = append(details, previewDetail(field, strconv.Itoa(plan.WithheldActivities)))
	}
	for _, state := range plan.Consent {
		details = append(details, previewDetail("consent_purpose", state.PurposeLabel))
		if sourceOnly {
			details = append(details, previewDetail("creation_source_consent", state.SourceState))
			continue
		}
		before := ""
		if state.TargetBefore != nil {
			before = *state.TargetBefore
		}
		details = append(details, previewDetail("promotion_consent_before", before), previewDetail("consent_result", state.TargetAfter))
	}
	for _, stop := range plan.Stops {
		details = append(details, previewDetail("promotion_restriction", stop.Kind), previewDetail("promotion_restriction_authority", stop.Authority))
		purpose := ""
		if stop.PurposeID != nil {
			purpose = stop.PurposeLabel
			if purpose == "" {
				return nil, apperrors.ErrInvalidArgument
			}
		}
		details = append(details, previewDetail("promotion_restriction_purpose", purpose))
	}
	links, err := r.promotionConsentLinkDetails(ctx, plan.ConsentLinks)
	if err != nil {
		return nil, err
	}
	details = append(details, links...)
	return details, nil
}

func (r approvalPreviewReader) promotionDealDetails(ctx context.Context, deal contacts.QualifyDealInput) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("promotion_deal_name", deal.Name)}
	for _, ref := range []struct {
		field string
		id    *ids.UUID
	}{{fieldPipelineID, deal.PipelineID}, {fieldStageID, deal.StageID}} {
		if ref.id == nil {
			return nil, apperrors.ErrInvalidArgument
		}
		value, err := r.referenceValue(ctx, ref.field, ref.id.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: ref.field, Value: value})
	}
	money := crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty}
	if deal.AmountMinor != nil && deal.Currency != nil {
		money = crmcontracts.ApprovalPreviewValue{Kind: "money", Text: strconv.FormatInt(*deal.AmountMinor, 10), Currency: deal.Currency}
	}
	details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_deal_amount", Value: money})
	owner := crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty}
	if deal.OwnerID != nil {
		var err error
		owner, err = r.referenceValue(ctx, bulkParamOwner, deal.OwnerID.String())
		if err != nil {
			return nil, err
		}
	}
	return append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_deal_owner", Value: owner}), nil
}

func (r approvalPreviewReader) promotionNewContactDetails(ctx context.Context, plan contacts.LeadContactChange) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	spec := plan.Create
	details = append(details, previewDetail("promotion_contact_name", spec.FullName), previewDetail("promotion_contact_source", spec.Source))
	if spec.Title != nil {
		details = append(details, previewDetail("promotion_contact_title", *spec.Title))
	}
	owner := crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty}
	if spec.OwnerID != nil {
		var err error
		owner, err = r.referenceValue(ctx, bulkParamOwner, spec.OwnerID.String())
		if err != nil {
			return nil, err
		}
	}
	details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_contact_owner", Value: owner})
	for _, email := range spec.Emails {
		details = append(details, previewDetail("promotion_contact_email", email.Email))
	}
	for _, phone := range spec.Phones {
		details = append(details, previewDetail("promotion_contact_phone", phone.Phone))
	}
	return details, nil
}

func (r approvalPreviewReader) promotionExistingContactDetails(ctx context.Context, plan contacts.LeadContactChange) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	contact, err := r.referenceValue(ctx, companionContactField, plan.ContactID.String())
	if err != nil {
		return nil, err
	}
	details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_existing_contact", Value: contact})
	after := plan.Patch.After()
	if title, ok := after[fieldTitle]; ok {
		raw, err := json.Marshal(title)
		if err != nil {
			return nil, err
		}
		value, err := r.value(ctx, fieldTitle, raw)
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "promotion_contact_title", Value: value})
	}
	if _, linked := after["converted_from_lead_id"]; linked {
		details = append(details, previewDetail("promotion_set_origin", "true"))
	}
	for _, phone := range plan.Phones {
		details = append(details, previewDetail("promotion_contact_phone", phone.Phone))
	}
	return details, nil
}
