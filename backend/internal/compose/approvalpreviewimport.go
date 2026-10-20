// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) importPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version = 2
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || *approval.TargetEntityType != "import_run" {
		return out, nil
	}
	run, err := (importHandlers{db: r.db}).stagedFor(ctx, *approval.TargetEntityId)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.State = previewIncompleteState
	if run.Mapping == nil || run.Report == nil || run.Report.ReviewPlan == nil || !run.Report.ReviewPlan.Complete {
		out.Conflicts = &[]crmcontracts.ApprovalPreviewConflict{{Kind: "import_scope_unavailable", BlocksApproval: true, Summary: "The complete row plan is unavailable. Review this import in Data import, split larger files or resolve overlapping rows, and run a new test before requesting approval."}}
		return out, nil
	}
	if run.Report.ReviewPlan.Version != 1 || len(run.Report.ReviewPlan.Rows) > migration.ReviewRowLimit {
		return out, nil
	}
	request, err := commercialRequest[struct {
		RunID   ids.UUID `json:"run_id"`
		Version *int     `json:"version"`
	}](approval)
	if err != nil || approval.ProposedChange == nil {
		return out, nil
	}
	if _, rest := (*approval.ProposedChange)["operation"]; rest {
		if !previewRouteMatches(*approval.ProposedChange, "commit_import", "import_run", ids.UUID(run.ID), "POST") {
			return out, nil
		}
		request.RunID = ids.UUID(run.ID)
	}
	if request.RunID != ids.UUID(run.ID) {
		return out, nil
	}
	if run.Status != migration.StatusAwaitingApproval || approval.TargetVersion == nil || *approval.TargetVersion != run.Version ||
		request.Version != nil && *request.Version != run.Version {
		out.State = previewStaleState
		return out, nil
	}
	err = database.WithWorkspaceSnapshot(ctx, r.db.Pool(), func(ctx context.Context) error {
		rows, err := r.importRows(ctx, run, out)
		if err != nil {
			return err
		}
		out.ImportRows, out.CurrentVersion, out.State = &rows, new(run.Version), previewReadyState
		return nil
	})
	if errors.Is(err, apperrors.ErrVersionSkew) {
		out.State = previewStaleState
		return out, nil
	}
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
