// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func enrichmentCommand(approval crmcontracts.Approval) (agents.EnrichCommand, error) {
	args, err := commercialArguments(approval)
	if err != nil {
		return agents.EnrichCommand{}, err
	}
	if approval.ProposedChange == nil || approval.TargetEntityId == nil || approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeCompany) {
		return agents.EnrichCommand{}, apperrors.ErrInvalidArgument
	}
	if args == nil {
		args = map[string]json.RawMessage{}
	}
	if operation, rest := (*approval.ProposedChange)["operation"]; rest {
		name, ok := operation.(string)
		if !ok {
			return agents.EnrichCommand{}, apperrors.ErrInvalidArgument
		}
		depth, ok := enrichOperationDepth[name]
		if !ok {
			return agents.EnrichCommand{}, apperrors.ErrInvalidArgument
		}
		args["depth"], err = json.Marshal(depth)
		if err != nil {
			return agents.EnrichCommand{}, err
		}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return agents.EnrichCommand{}, err
	}
	cmd, err := agents.DecodeEnrichCommand(raw)
	if err != nil {
		return agents.EnrichCommand{}, apperrors.ErrInvalidArgument
	}
	target := ids.UUID(*approval.TargetEntityId)
	if !cmd.CompanyID.IsZero() && cmd.CompanyID != target {
		return agents.EnrichCommand{}, apperrors.ErrInvalidArgument
	}
	cmd.CompanyID = target
	return cmd, nil
}

func (r approvalPreviewReader) enrichmentPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	cmd, err := enrichmentCommand(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if err := agents.NewEnrichCall(r.records, nil, cmd).Guards(ctx); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	target, err := r.enrichmentTarget(ctx, cmd)
	if previewUnavailable(err) || errors.Is(err, contacts.ErrNoEnrichTarget) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("target_url", target), previewDetail("enrichment_depth", string(cmd.Depth))}
	out.Details = &details
	out.Effects = &[]string{"authorize_website_read"}
	out.State = previewReadyState
	if err := r.currentEvidence(ctx, approval, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r approvalPreviewReader) enrichmentTarget(ctx context.Context, cmd agents.EnrichCommand) (string, error) {
	store := contacts.NewStore(r.db)
	company := ids.From[ids.CompanyKind](cmd.CompanyID)
	if cmd.Depth == agents.EnrichDepthTechnical {
		domain, found, err := store.TechnicalDomain(ctx, company)
		if err != nil {
			return "", err
		}
		if !found {
			return "", contacts.ErrNoEnrichTarget
		}
		return domain, nil
	}
	return store.ResolveEnrichTargetURL(ctx, company, cmd.URL)
}
