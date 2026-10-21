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

func (r approvalPreviewReader) mergeRelationshipDetails(ctx context.Context, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, item := range scope.Relationships {
		relation, err := r.mergeRelationship(ctx, item.Relationship)
		if err != nil {
			return nil, err
		}
		details = append(details, relation...)
		details = append(details, mergeBooleanDetail("merge_relationship_retired", item.Retired), mergeBooleanDetail("merge_relationship_primary", item.WillBePrimary))
	}
	for _, observation := range scope.Observations {
		relation, err := r.mergeRelationship(ctx, observation.Relationship)
		if err != nil {
			return nil, err
		}
		details = append(details, relation...)
		details = append(details, previewMoment("merge_observation_before", observation.Before), previewMoment("merge_observation_after", &observation.After))
	}
	for _, support := range scope.EmploymentSupport {
		for _, relation := range []crmcontracts.Relationship{support.Source, support.Target} {
			fields, err := r.mergeRelationship(ctx, relation)
			if err != nil {
				return nil, err
			}
			details = append(details, fields...)
		}
		for _, count := range []struct {
			field string
			value int64
		}{{"merge_support_resolutions", support.Resolutions}, {"merge_support_fields", support.AppliedFields}, {"merge_support_collapsed", support.CollapsedFields}} {
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: count.field, Value: crmcontracts.ApprovalPreviewValue{Kind: previewValueNumber, Text: strconv.FormatInt(count.value, 10)}})
		}
	}
	return details, nil
}

func (r approvalPreviewReader) mergeRelationship(ctx context.Context, relation crmcontracts.Relationship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{mergeIdentityDetail("merge_relationship_kind", string(relation.Kind), ids.UUID(relation.Id))}
	for _, ref := range []struct {
		field string
		id    *openapi_types.UUID
	}{{companionContactField, relation.ContactId}, {refColCompany, relation.CompanyId}, {companionContactField, relation.CounterpartyContactId}, {refColCompany, relation.CounterpartyCompanyId}, {dealIDField, relation.DealId}, {refColProject, relation.ProjectId}} {
		if ref.id == nil {
			continue
		}
		value, err := r.referenceValue(ctx, ref.field, ref.id.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: ref.field, Value: value})
	}
	if relation.Role != nil {
		details = append(details, previewDetail("relationship_role", *relation.Role))
	}
	return details, nil
}

func (r approvalPreviewReader) mergeParticipantDetails(ctx context.Context, scope contacts.MergeSurvivorship) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, participant := range scope.Participants {
		activity, err := r.referenceValue(ctx, "activity_id", participant.ActivityID.String())
		if err != nil {
			return nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "merge_participant_activity", Value: activity}, mergeIdentityDetail("merge_participant_role", participant.Role, participant.ID), mergeBooleanDetail("merge_participant_collapsed", participant.Collapses))
		if participant.Address != nil {
			details = append(details, previewDetail("merge_participant_address", *participant.Address))
		}
	}
	for _, intro := range scope.Introductions {
		details = append(details, mergeIdentityDetail("merge_introduction_before", intro.StatusBefore, intro.ID), previewDetail("merge_introduction_after", intro.StatusAfter))
		for _, ref := range []struct {
			field string
			id    *ids.UUID
		}{{"merge_introduction_contact_before", &intro.ContactBefore}, {"merge_introduction_contact_after", &intro.ContactAfter}, {"merge_introduction_through_before", intro.ThroughBefore}, {"merge_introduction_through_after", intro.ThroughAfter}} {
			if ref.id == nil {
				continue
			}
			value, err := r.referenceValue(ctx, companionContactField, ref.id.String())
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: ref.field, Value: value})
		}
		if intro.CancellationReason != "" {
			details = append(details, previewDetail("merge_introduction_cancellation", intro.CancellationReason))
		}
	}
	return details, nil
}
