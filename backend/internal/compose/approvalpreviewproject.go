// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) projectPhaseChanges(ctx context.Context, approval crmcontracts.Approval, current, changes map[string]json.RawMessage) error {
	args, err := commercialArguments(approval)
	if err != nil {
		return err
	}
	var phase string
	if err := json.Unmarshal(args["to_phase"], &phase); err != nil {
		return apperrors.ErrInvalidArgument
	}
	var reason *string
	if raw, present := args["reason"]; present {
		if err := json.Unmarshal(raw, &reason); err != nil {
			return apperrors.ErrInvalidArgument
		}
	}
	preview, err := ProjectsStoreOver(r.db).PreviewAdvanceProjectPhase(ctx, ids.From[ids.ProjectKind](ids.UUID(*approval.TargetEntityId)), projects.AdvanceProjectPhaseInput{
		ToPhase: phase, Reason: reason, IfVersion: new(int64(*approval.TargetVersion)),
	})
	if err != nil {
		return err
	}
	return applyPreviewChanges(current, changes, preview.Before, preview.After, "")
}
