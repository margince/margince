// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func contactCreationRequest(approval crmcontracts.Approval) (contacts.CreateContactInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateContactRequest](approval, string(recordTypeContact))
	if err != nil {
		return contacts.CreateContactInput{}, err
	}
	if tool {
		req.Source = agents.ToolSource
	}
	return contacts.ContactCreationInput(req)
}

func (r approvalPreviewReader) contactCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	in, err := contactCreationRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	creator, err := r.creationAuthority(ctx, approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	consents := consent.NewStore(r.db)
	store := contacts.NewStore(r.db).WithFieldCatalog(customfields.NewService(r.db.Pool(), nil)).WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
	plan, err := store.PreviewContactCreationForActor(ctx, in, creator)
	if refused, ok := errors.AsType[*values.ParseError](err); ok && refused.Code == contacts.MachineAddressCode {
		out.State = previewIncompleteState
		out.Conflicts = &[]crmcontracts.ApprovalPreviewConflict{{Kind: refused.Code, Summary: refused.Message, BlocksApproval: true}}
		return out, nil
	}
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, effects, err := r.contactCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) contactCreationDetails(ctx context.Context, plan contacts.ContactCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	in := plan.Input
	fields := map[string]any{fieldFullName: in.FullName, previewFieldFirstName: in.FirstName, previewFieldLastName: in.LastName, fieldTitle: in.Title, bulkParamOwner: in.OwnerID, fieldSource: in.Source, previewFieldSourceSystem: in.SourceSystem, previewFieldSourceAuthorName: in.Author.AuthorName, previewFieldSourceAuthorID: in.Author.AuthorID, "creation_acquisition": plan.Acquisition.Kind}
	maps.Copy(fields, plan.CustomFields)
	maps.Copy(fields, creationAddressFields(in.Address))
	if err := creationFieldsVisible(ctx, string(recordTypeContact), map[string]any{addressField: in.Address, previewFieldSocial: in.Social, previewFieldEmails: in.Emails, "phones": in.Phones, "acquisition": plan.Acquisition}); err != nil {
		return nil, nil, err
	}
	details, err := r.creationFieldDetails(ctx, string(recordTypeContact), fields)
	if err != nil {
		return nil, nil, err
	}
	if plan.Acquisition.OccurredAt != nil {
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_acquired_at", Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueDate, Text: plan.Acquisition.OccurredAt.Format(time.RFC3339)}})
	}
	communications, err := r.contactCreationChannels(ctx, plan)
	if err != nil {
		return nil, nil, err
	}
	details = append(details, communications...)
	effects := []string{"create_contact_record", "create_workspace_visible", "record_contact_acquisition"}
	if plan.ReviewContactID != nil {
		value, err := r.referenceValue(ctx, companionContactField, plan.ReviewContactID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: previewCreationDuplicate, Value: value})
		effects = append(effects, "queue_duplicate_review")
	}
	for _, promotion := range plan.Promotions {
		value, err := r.referenceValue(ctx, "lead_id", promotion.LeadID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_promoted_lead", Value: value})
		changes, err := r.creationFieldDetails(ctx, string(recordTypeContact), promotion.Patch.After())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, changes...)
		for _, phone := range promotion.Phones {
			details = append(details, previewDetail("promotion_contact_phone", phone.Phone))
		}
		carried, err := r.promotionCarryDetails(ctx, promotion.SourceCarry, true)
		if err != nil {
			return nil, nil, err
		}
		details = append(details, carried...)
	}
	if len(plan.Promotions) > 0 {
		effects = append(effects, "create_promote_matching_leads")
	}
	return details, effects, nil
}

func (r approvalPreviewReader) contactCreationChannels(ctx context.Context, plan contacts.ContactCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var details []crmcontracts.ApprovalPreviewDetail
	for _, email := range plan.Input.Emails {
		fields := map[string]any{fieldEmail: email.Email, "creation_email_type": email.EmailType, previewFieldCreationPrimary: email.IsPrimary, previewCreationPosition: email.Position, "creation_email_correspondence": !email.VouchedNotCorresponded}
		rows, err := r.creationChannelDetails(ctx, fields)
		if err != nil {
			return nil, err
		}
		details = append(details, rows...)
	}
	for _, phone := range plan.Input.Phones {
		rows, err := r.creationChannelDetails(ctx, map[string]any{fieldPhone: phone.Phone, "creation_phone_type": phone.PhoneType, previewFieldCreationPrimary: phone.IsPrimary, previewCreationPosition: phone.Position})
		if err != nil {
			return nil, err
		}
		details = append(details, rows...)
	}
	for _, social := range plan.Social {
		details = append(details, previewDetail("creation_social_platform", social.Platform), previewDetail("creation_social_handle", social.Handle))
	}
	return details, nil
}

func (r approvalPreviewReader) creationChannelDetails(ctx context.Context, fields map[string]any) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var details []crmcontracts.ApprovalPreviewDetail
	for _, field := range []string{fieldEmail, fieldPhone, "creation_email_type", "creation_phone_type", previewFieldCreationPrimary, previewCreationPosition, "creation_email_correspondence"} {
		scalar, ok := fields[field]
		if !ok {
			continue
		}
		raw, err := json.Marshal(scalar)
		if err != nil {
			return nil, err
		}
		value, err := r.value(ctx, "", raw)
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: value})
	}
	return details, nil
}
