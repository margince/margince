// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
)

func contactCreationRequest(approval crmcontracts.Approval) (contacts.CreateContactInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateContactRequest](approval, "contact")
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
	fields := map[string]any{"full_name": in.FullName, "first_name": in.FirstName, "last_name": in.LastName, "title": in.Title, "owner_id": in.OwnerID, "source": in.Source, "source_system": in.SourceSystem, "source_author_name": in.Author.AuthorName, "source_author_id": in.Author.AuthorID, "creation_acquisition": plan.Acquisition.Kind}
	maps.Copy(fields, plan.CustomFields)
	maps.Copy(fields, creationAddressFields(in.Address))
	if err := creationFieldsVisible(ctx, "contact", map[string]any{"address": in.Address, "social": in.Social, "emails": in.Emails, "phones": in.Phones, "acquisition": plan.Acquisition}); err != nil {
		return nil, nil, err
	}
	details, err := r.creationFieldDetails(ctx, "contact", fields)
	if err != nil {
		return nil, nil, err
	}
	if plan.Acquisition.OccurredAt != nil {
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_acquired_at", Value: crmcontracts.ApprovalPreviewValue{Kind: "date", Text: plan.Acquisition.OccurredAt.Format(time.RFC3339)}})
	}
	communications, err := r.contactCreationChannels(ctx, plan)
	if err != nil {
		return nil, nil, err
	}
	details = append(details, communications...)
	effects := []string{"create_contact_record", "create_workspace_visible", "record_contact_acquisition"}
	if plan.ReviewContactID != nil {
		value, err := r.referenceValue(ctx, "contact_id", plan.ReviewContactID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_duplicate", Value: value})
		effects = append(effects, "queue_duplicate_review")
	}
	for _, promotion := range plan.Promotions {
		value, err := r.referenceValue(ctx, "lead_id", promotion.LeadID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_promoted_lead", Value: value})
		changes, err := r.creationFieldDetails(ctx, "contact", promotion.Patch.After())
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
		fields := map[string]any{"email": email.Email, "creation_email_type": email.EmailType, "creation_primary": email.IsPrimary, "creation_position": email.Position, "creation_email_correspondence": !email.VouchedNotCorresponded}
		rows, err := r.creationChannelDetails(ctx, fields)
		if err != nil {
			return nil, err
		}
		details = append(details, rows...)
	}
	for _, phone := range plan.Input.Phones {
		rows, err := r.creationChannelDetails(ctx, map[string]any{"phone": phone.Phone, "creation_phone_type": phone.PhoneType, "creation_primary": phone.IsPrimary, "creation_position": phone.Position})
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
	for _, field := range []string{"email", "phone", "creation_email_type", "creation_phone_type", "creation_primary", "creation_position", "creation_email_correspondence"} {
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
