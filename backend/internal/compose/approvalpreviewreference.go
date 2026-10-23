// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (r approvalPreviewReader) referenceValue(ctx context.Context, field, text string) (crmcontracts.ApprovalPreviewValue, error) {
	var value crmcontracts.ApprovalPreviewValue
	id, err := ids.Parse(text)
	if err != nil {
		return value, apperrors.ErrInvalidArgument
	}
	entityType := previewReferenceType(field)
	labels, cached := r.labels[entityType]
	if !cached {
		labels, err = r.names.Labels(ctx, entityType, []ids.UUID{id})
		if err != nil {
			return value, err
		}
	}
	label, found := labels[id]
	if !found || strings.TrimSpace(label) == "" {
		return value, apperrors.ErrNotFound
	}
	kind := previewValueReference
	if field == bulkParamOwner {
		kind = ScopeKindOwner
	}
	if field == fieldStageID {
		kind = dealTargetStage
	}
	return crmcontracts.ApprovalPreviewValue{Kind: kind, Text: approvals.SanitizeSummary(label), EntityType: &entityType, EntityId: new(openapi_types.UUID(id))}, nil
}

func previewReferenceType(field string) string {
	switch field {
	case previewFieldTemplateID:
		return string(recordTypeOfferTemplate)
	case bulkParamOwner:
		return entityUser
	case previewFieldBuyerCompanyID, refColParentCompany, refColPartnerCompany:
		return string(recordTypeCompany)
	default:
		return strings.TrimSuffix(field, "_id")
	}
}

func (r approvalPreviewReader) referenceNames(ctx context.Context, current, changes map[string]json.RawMessage) (map[string]map[ids.UUID]string, error) {
	references := map[string][]ids.UUID{}
	for field, after := range changes {
		if !strings.HasSuffix(field, "_id") {
			continue
		}
		for _, raw := range []json.RawMessage{current[field], after} {
			if len(raw) == 0 || string(raw) == jsonNull {
				continue
			}
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return nil, apperrors.ErrInvalidArgument
			}
			id, err := ids.Parse(text)
			if err != nil {
				return nil, apperrors.ErrInvalidArgument
			}
			entityType := previewReferenceType(field)
			references[entityType] = append(references[entityType], id)
		}
	}
	labels := make(map[string]map[ids.UUID]string, len(references))
	for entityType, ids := range references {
		names, err := r.names.Labels(ctx, entityType, ids)
		if err != nil {
			return nil, err
		}
		labels[entityType] = names
	}
	return labels, nil
}

func previewDetail(field, value string) crmcontracts.ApprovalPreviewDetail {
	kind := previewValueText
	if value == "" {
		kind = previewValueEmpty
	}
	return crmcontracts.ApprovalPreviewDetail{Field: field, Value: crmcontracts.ApprovalPreviewValue{Kind: kind, Text: value}}
}

func (r approvalPreviewReader) previewFiles(ctx context.Context, attachments *[]openapi_types.UUID) ([]crmcontracts.ApprovalPreviewFile, error) {
	requested := []ids.UUID{}
	if attachments != nil {
		for _, id := range *attachments {
			requested = append(requested, ids.UUID(id))
		}
	}
	files, err := r.activities.PreviewSendAttachments(ctx, requested)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if file.Filename == "" {
			return nil, apperrors.ErrInvalidArgument
		}
	}
	return activities.ApprovalPreviewFiles(files), nil
}

func (r approvalPreviewReader) previewLinkedRecords(ctx context.Context, links []activities.ActivityLinkInput) ([]crmcontracts.ApprovalPreviewDetail, error) {
	details := make([]crmcontracts.ApprovalPreviewDetail, 0, len(links))
	for _, link := range links {
		record, err := r.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityType(link.EntityType), ID: link.EntityID})
		if previewUnavailable(err) {
			return nil, apperrors.ErrInvalidArgument
		}
		if err != nil {
			return nil, err
		}
		name, err := r.names.Labels(ctx, string(link.EntityType), []ids.UUID{record.Ref.ID})
		if err != nil {
			return nil, err
		}
		label, ok := name[record.Ref.ID]
		if !ok || label == "" {
			return nil, apperrors.ErrInvalidArgument
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "filed_under", Value: crmcontracts.ApprovalPreviewValue{
			Kind: previewValueReference, Text: label, EntityType: new(string(link.EntityType)), EntityId: new(openapi_types.UUID(link.EntityID)),
		}})
	}
	return details, nil
}

func previewLinkDetail(field, value string) crmcontracts.ApprovalPreviewDetail {
	detail := previewDetail(field, value)
	if value != "" {
		detail.Value.Kind = "link"
	}
	return detail
}
