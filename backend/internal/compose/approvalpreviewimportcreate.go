// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (r approvalPreviewReader) importCreationDetails(ctx, execCtx context.Context, writer *csvWriters, row migration.Row, typed map[string]any, actor principal.Principal) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	fields := textFields(row.Fields)
	source := writer.provenanceOf(row.ExternalID)
	switch writer.object {
	case migration.ObjectCompany:
		in := companyCreateFrom(fields, typed, writer.sourceSystem, source)
		if err := writer.resolveSourceAuthor(execCtx, &in.Author); err != nil {
			return nil, nil, err
		}
		plan, err := writer.contacts.PreviewCompanyCreationForActor(contacts.WithBulkWrite(ctx), in, actor)
		if err != nil {
			return nil, nil, err
		}
		return r.companyCreationDetails(ctx, plan)
	case migration.ObjectContact:
		in := contactCreateFrom(fields, typed, writer.sourceSystem, source)
		if err := writer.resolveSourceAuthor(execCtx, &in.Author); err != nil {
			return nil, nil, err
		}
		plan, err := writer.contacts.PreviewContactCreationForActor(contacts.WithBulkWrite(ctx), in, actor)
		if err != nil {
			return nil, nil, err
		}
		return r.contactCreationDetails(ctx, plan)
	case migration.ObjectLead:
		in := leadCreateFrom(fields, typed, writer.sourceSystem, row.ExternalID, source)
		if err := writer.resolveSourceAuthor(execCtx, &in.Author); err != nil {
			return nil, nil, err
		}
		plan, err := writer.contacts.PreviewLeadCreationForActor(contacts.WithBulkWrite(ctx), in, actor)
		if err != nil {
			return nil, nil, err
		}
		return r.leadCreationDetails(ctx, plan)
	case migration.ObjectDeal:
		return r.importDealDetails(ctx, writer, row, typed, actor)
	case migration.ObjectActivity:
		return r.importActivityDetails(ctx, execCtx, writer, row, typed)
	default:
		return nil, nil, apperrors.ErrInvalidArgument
	}
}

func (r approvalPreviewReader) importActivityDetails(ctx, execCtx context.Context, writer *csvWriters, row migration.Row, typed map[string]any) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	in, why := writer.activityBirthOf(row, typed)
	if why.Code != "" {
		return nil, nil, apperrors.ErrInvalidArgument
	}
	owner := in.Author.AuthorID
	if err := writer.resolveSourceAuthor(execCtx, &in.Author); err != nil {
		return nil, nil, err
	}
	if owner != nil {
		in.Author.AuthorID = owner
	}
	fields := map[string]any{
		"kind": in.Kind, "subject": in.Subject, "body": in.Body, "occurred_at": in.OccurredAt,
		"direction": in.Direction, "duration_seconds": in.DurationSeconds, "meeting_status": in.MeetingStatus,
		"source": in.Source, "source_system": in.SourceSystem,
		"source_author_id": in.Author.AuthorID, "source_author_name": in.Author.AuthorName,
	}
	details, err := r.creationFieldDetails(ctx, "activity", fields)
	if err != nil {
		return nil, nil, err
	}
	for _, link := range in.Links {
		value, err := r.referenceValue(ctx, link.EntityType+"_id", link.EntityID.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: link.EntityType + "_id", Value: value})
	}
	return details, []string{"create_activity_record"}, nil
}

func (r approvalPreviewReader) importDealDetails(ctx context.Context, writer *csvWriters, row migration.Row, typed map[string]any, actor principal.Principal) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	in, terminal := writer.dealBirthOf(row, typed)
	plan, err := writer.deals.PreviewImportDealCreationForActor(ctx, in.Deal, actor, terminal)
	if err != nil {
		return nil, nil, err
	}
	details, err := r.dealCreationDetails(ctx, plan)
	if err != nil {
		return nil, nil, err
	}
	effects := []string{"create_deal_record"}
	if terminal != nil {
		fields := map[string]any{"closed_at": terminal.ClosedAt, "lost_reason": terminal.LostReason}
		choice, problem := writer.dealStages.resolve(textFields(row.Fields)[dealTargetPipeline], textFields(row.Fields)[dealTargetStage])
		if problem == nil && choice.semantic == crmcontracts.StageSemanticWon {
			fields["won_without_contract_reason"] = importedWinReason
		}
		closed, err := r.creationFieldDetails(ctx, "deal", fields)
		if err != nil {
			return nil, nil, err
		}
		details = append(details, closed...)
		effects = append(effects, "import_close_deal")
	}
	return details, effects, nil
}
