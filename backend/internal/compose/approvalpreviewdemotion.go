// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) demotionPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	id, reason, err := demotionPreviewRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	store := contacts.NewStore(r.db)
	lead, err := store.GetLead(ctx, id, storekit.IncludeArchived)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if lead.Version == nil {
		return out, nil
	}
	out.CurrentVersion = new(int(*lead.Version))
	version := int64(*approval.TargetVersion)
	plan, err := store.PreviewLeadDemotion(ctx, id, contacts.OnlyAtVersion(&version))
	if errors.Is(err, apperrors.ErrVersionSkew) {
		out.State = previewStaleState
		return out, nil
	}
	if previewUnavailable(err) || errors.Is(err, apperrors.ErrConflict) {
		return out, nil
	}
	if _, ok := errors.AsType[*contacts.NotPromotedError](err); ok {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	contact, err := r.referenceValue(ctx, companionContactField, plan.ContactID.String())
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details = &[]crmcontracts.ApprovalPreviewDetail{previewDetail("demotion_reason", reason), {Field: "demotion_contact", Value: contact}}
	effect := "restore_lead_preserve_contact"
	if plan.Unwind == crmcontracts.DemoteLeadResponseUnwindDemoteUnwindReversed {
		effect = "restore_lead_archive_contact"
	}
	out.Effects = &[]string{effect}
	out.State = previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func demotionPreviewRequest(approval crmcontracts.Approval) (ids.LeadID, string, error) {
	in, err := commercialRequest[struct {
		LeadID ids.UUID `json:"lead_id"`
		Reason string   `json:"reason"`
	}](approval)
	if previewUnavailable(err) || *approval.TargetEntityType != string(recordTypeLead) {
		return ids.LeadID{}, "", apperrors.ErrInvalidArgument
	}
	if err != nil {
		return ids.LeadID{}, "", err
	}
	id := ids.From[ids.LeadKind](ids.UUID(*approval.TargetEntityId))
	if !in.LeadID.IsZero() && in.LeadID != id.UUID {
		return ids.LeadID{}, "", apperrors.ErrInvalidArgument
	}
	if err := agents.RequireDemotionReason(in.Reason); err != nil {
		return ids.LeadID{}, "", apperrors.ErrInvalidArgument
	}
	return id, in.Reason, nil
}
