// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (r approvalPreviewReader) recordArchiveDetails(ctx context.Context, approval crmcontracts.Approval) ([]crmcontracts.ApprovalPreviewDetail, error) {
	if approval.TargetEntityId == nil || approval.TargetVersion == nil {
		return nil, apperrors.ErrInvalidArgument
	}
	scope, err := r.recordArchiveScope(ctx, approval)
	if err != nil {
		return nil, err
	}
	var out []crmcontracts.ApprovalPreviewDetail
	for _, id := range scope.Relationships {
		record, err := r.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityRelationship, ID: id})
		if err != nil {
			return nil, err
		}
		var edge crmcontracts.Relationship
		if err := json.Unmarshal(record.Fields, &edge); err != nil {
			return nil, err
		}
		if *approval.TargetEntityType == "contact" || *approval.TargetEntityType == "company" {
			target, counterparty := edge.ContactId, edge.CounterpartyContactId
			if *approval.TargetEntityType == "company" {
				target, counterparty = edge.CompanyId, edge.CounterpartyCompanyId
			}
			if (target == nil || *target != *approval.TargetEntityId) && (counterparty == nil || *counterparty != *approval.TargetEntityId) {
				return nil, apperrors.ErrInvalidArgument
			}
			details, err := r.relationshipArchiveDetails(ctx, edge)
			if err != nil {
				return nil, err
			}
			out = append(out, details...)
			continue
		}
		target := edge.DealId
		var kind crmcontracts.RelationshipKind = "deal_stakeholder"
		if *approval.TargetEntityType == "project" {
			target, kind = edge.ProjectId, "project_stakeholder"
		}
		if edge.ContactId == nil || target == nil || *target != *approval.TargetEntityId || edge.Kind != kind {
			return nil, apperrors.ErrInvalidArgument
		}
		contact, err := r.referenceValue(ctx, "contact_id", edge.ContactId.String())
		if err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.ApprovalPreviewDetail{Field: "archive_stakeholder", Value: contact})
		if edge.Role != nil {
			out = append(out, crmcontracts.ApprovalPreviewDetail{Field: "archive_role", Value: crmcontracts.ApprovalPreviewValue{Kind: "text", Text: *edge.Role}})
		}
	}
	for _, group := range []struct {
		kind string
		ids  []ids.UUID
	}{{"list", scope.Lists}, {"tag", scope.Tags}} {
		for _, id := range group.ids {
			value, err := r.referenceValue(ctx, group.kind+"_id", id.String())
			if err != nil {
				return nil, err
			}
			out = append(out, crmcontracts.ApprovalPreviewDetail{Field: "archive_" + group.kind, Value: value})
		}
	}
	return out, nil
}

func (r approvalPreviewReader) recordArchiveScope(ctx context.Context, approval crmcontracts.Approval) (storekit.ArchiveScope, error) {
	if approval.TargetEntityType == nil || approval.TargetEntityId == nil || approval.TargetVersion == nil {
		return storekit.ArchiveScope{}, apperrors.ErrInvalidArgument
	}
	id, version := ids.UUID(*approval.TargetEntityId), int64(*approval.TargetVersion)
	switch *approval.TargetEntityType {
	case "company":
		return contacts.NewStore(r.db).PreviewCompanyArchive(ctx, ids.From[ids.CompanyKind](id), &version)
	case "contact":
		return contacts.NewStore(r.db).PreviewContactArchive(ctx, ids.From[ids.ContactKind](id), &version)
	case "deal":
		return r.deals.PreviewDealArchive(ctx, ids.From[ids.DealKind](id), &version)
	case "project":
		return ProjectsStoreOver(r.db).PreviewProjectArchive(ctx, ids.From[ids.ProjectKind](id), &version)
	default:
		return storekit.ArchiveScope{}, apperrors.ErrInvalidArgument
	}
}
