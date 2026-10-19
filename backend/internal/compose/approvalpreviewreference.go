// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	kind := "reference"
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
	case bulkParamOwner:
		return entityUser
	case "parent_company_id", "partner_company_id":
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
