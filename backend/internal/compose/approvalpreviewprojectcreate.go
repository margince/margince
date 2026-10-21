// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"maps"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/projects"
)

func projectCreationRequest(approval crmcontracts.Approval) (projects.CreateProjectInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateProjectRequest](approval, string(recordTypeProject))
	if err != nil {
		return projects.CreateProjectInput{}, err
	}
	if tool {
		req.Source = agents.ToolSource
	}
	return projects.ProjectCreationInput(req)
}

func (r approvalPreviewReader) projectCreationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	in, err := projectCreationRequest(approval)
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
	plan, err := ProjectsStoreOver(r.db).PreviewProjectCreationForActor(ctx, in, creator)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, err := r.projectCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &[]string{"create_project_record", "assign_project_key"}, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) projectCreationDetails(ctx context.Context, in projects.CreateProjectInput) ([]crmcontracts.ApprovalPreviewDetail, error) {
	fields := map[string]any{
		previewFieldName: in.Name, refColCompany: in.CompanyID, bulkParamOwner: in.OwnerID,
		descriptionField: in.Description, previewFieldStartedAt: in.StartedAt, previewFieldTargetEndDate: in.TargetEndDate,
		fieldPhase: projects.PhaseInitiative, fieldSource: in.Source, previewFieldSourceSystem: in.SourceSystem,
		previewFieldSourceAuthorID: in.Author.AuthorID, previewFieldSourceAuthorName: in.Author.AuthorName,
	}
	maps.Copy(fields, in.CustomFields)
	return r.creationFieldDetails(ctx, string(recordTypeProject), fields)
}
