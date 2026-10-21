// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mergeIdentityDetail(field, label string, id ids.UUID) crmcontracts.ApprovalPreviewDetail {
	detail := previewDetail(field, label)
	detail.Value.EntityId = new(openapi_types.UUID(id))
	return detail
}

func mergeBooleanDetail(field string, value bool) crmcontracts.ApprovalPreviewDetail {
	return crmcontracts.ApprovalPreviewDetail{Field: field, Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueBoolean, Text: strconv.FormatBool(value)}}
}

func (r approvalPreviewReader) mergeLinkDetails(ctx context.Context, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, link := range scope.Links {
		value, err := r.referenceValue(ctx, link.Kind+"_id", link.ID.String())
		if err != nil {
			return nil, err
		}
		field := "merge_move_link"
		if link.Collapsed {
			field = "merge_collapse_link"
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: field, Value: value})
		if link.Note != nil {
			details = append(details, previewDetail("merge_link_note", *link.Note))
		}
	}
	for _, address := range scope.Addresses {
		details = append(details, mergeIdentityDetail("merge_address", address.Value, address.ID), previewDetail("merge_address_kind", address.Kind), previewDetail("merge_address_usage", address.Usage), mergeBooleanDetail("merge_address_primary_before", address.WasPrimary), mergeBooleanDetail("merge_address_primary_after", address.WillBePrimary))
	}
	for _, social := range scope.Social {
		details = append(details, mergeIdentityDetail("merge_social_handle", social.SourceHandle, social.SourceID), previewDetail("merge_social_platform", social.Platform))
		if social.TargetHandle != nil {
			details = append(details, previewDetail("merge_social_survivor", *social.TargetHandle))
		}
	}
	for _, identity := range scope.ChannelIdentities {
		details = append(details, mergeIdentityDetail("merge_channel_account", identity.Account, identity.ID), previewDetail("merge_channel_provider", identity.Provider), mergeBooleanDetail("merge_channel_blocked", identity.Blocked))
		if identity.Username != nil {
			details = append(details, previewDetail("merge_channel_username", *identity.Username))
		}
	}
	return details, nil
}

func (r approvalPreviewReader) mergeConversationDetails(ctx context.Context, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, claim := range scope.Claims {
		source, err := r.referenceValue(ctx, "activity_id", claim.SourceActivityId.String())
		if err != nil {
			return nil, err
		}
		details = append(details, mergeIdentityDetail("merge_claim", claim.Body, ids.UUID(claim.Id)), crmcontracts.ApprovalPreviewDetail{Field: "merge_claim_source", Value: source}, previewDetail("merge_claim_quote", claim.SourceQuote))
	}
	for _, commitment := range scope.Commitments {
		details = append(details, mergeIdentityDetail("merge_commitment", commitment.Label, ids.UUID(commitment.Id)))
	}
	for _, notice := range scope.Notices {
		details = append(details, mergeIdentityDetail("merge_notice", notice.Subject, ids.UUID(notice.Id)))
	}
	for _, handoff := range scope.Handoffs {
		details = append(details, mergeIdentityDetail("merge_handoff", handoff.Note, handoff.ID))
		for _, actor := range []struct {
			field string
			id    *ids.UUID
		}{{"merge_handoff_sender", &handoff.SubmittedBy}, {"merge_handoff_recipient", handoff.AssignedTo}} {
			if actor.id == nil {
				continue
			}
			value, err := r.referenceValue(ctx, bulkParamOwner, actor.id.String())
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: actor.field, Value: value})
		}
	}
	return details, nil
}
