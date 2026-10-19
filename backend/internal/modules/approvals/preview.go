// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type PreviewContextError struct{ State string }

func (e *PreviewContextError) Error() string {
	return "approval context is " + e.State
}

func (e *PreviewContextError) MessageFault() (code, message string) {
	switch e.State {
	case "incomplete":
		return "approval_preview_incomplete", "The full change cannot be shown. Resolve the missing context before approving."
	case "conflict":
		return "approval_preview_conflict", "Resolve the conflicting context before approving this change."
	default:
		return "approval_preview_unavailable", "The context needed to review this change is unavailable. Check access and the source before approving."
	}
}

// WithPreview connects decision context to the authorized readers of its records.
func (s *Service) WithPreview(read func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error)) *Service {
	s.preview = read
	return s
}

func (s *Service) checkPreview(ctx context.Context, a row, edited json.RawMessage) error {
	if s.preview == nil {
		return nil
	}
	proposal := wire(a, s.now())
	if len(edited) > 0 {
		change, err := diffhash.DecodeObject(edited)
		if err != nil {
			return &InvalidEditError{Cause: err}
		}
		proposal.ProposedChange = &change
	}
	preview, err := s.proposalPreview(ctx, a, proposal)
	if err != nil {
		return err
	}
	return PreviewRefusal(preview)
}

// PreviewRefusal keeps decision gates and action affordances on the same context verdict.
// A nil preview belongs to a kind whose own decision checks govern it.
func PreviewRefusal(preview *crmcontracts.ApprovalPreview) error {
	if preview == nil {
		return nil
	}
	if preview.State == "stale" {
		return apperrors.ErrVersionSkew
	}
	if preview.Version != 1 || preview.State != "ready" {
		return &PreviewContextError{State: preview.State}
	}
	if preview.Conflicts != nil {
		for _, conflict := range *preview.Conflicts {
			if conflict.BlocksApproval {
				return &PreviewContextError{State: "conflict"}
			}
		}
	}
	return nil
}

func (s *Service) readDecisionContext(ctx context.Context, rows []crmcontracts.Approval, subjects []row) error {
	if err := s.readAssignmentStates(ctx, rows); err != nil {
		return err
	}
	if s.preview == nil {
		return nil
	}
	for i := range rows {
		preview, err := s.proposalPreview(ctx, subjects[i], rows[i])
		if err != nil {
			return err
		}
		rows[i].Preview = preview
	}
	return nil
}

func (s *Service) proposalPreview(ctx context.Context, subject row, proposal crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	preview, err := s.preview(ctx, proposal)
	if err != nil || preview == nil {
		return preview, err
	}
	defer func() {
		if preview.State != "ready" {
			preview.Changes = []crmcontracts.ApprovalPreviewChange{}
			preview.Effects = nil
		}
	}()
	if (subject.Kind == "advance_deal" || subject.Kind == "progress_deal") && !stagePreviewPinned(subject, proposal) {
		preview.State = "incomplete"
		return preview, nil
	}
	if subject.CoTargetVersion == nil {
		return preview, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return checkPin(ctx, tx, subject.CoTargetType, subject.CoTargetID, subject.CoTargetVersion, "second record")
	})
	if errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) {
		preview.State = "stale"
		return preview, nil
	}
	return preview, err
}

func stagePreviewPinned(subject row, proposal crmcontracts.Approval) bool {
	if subject.CoTargetVersion == nil || subject.CoTargetType == nil || *subject.CoTargetType != "stage" || subject.CoTargetID == nil {
		return false
	}
	raw, err := json.Marshal(proposal.ProposedChange)
	if err != nil {
		return false
	}
	type move struct {
		ToStageID ids.UUID `json:"to_stage_id"`
	}
	var payload struct {
		move
		Body *move `json:"body"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	destination := payload.ToStageID
	if payload.Body != nil {
		destination = payload.Body.ToStageID
	}
	return !destination.IsZero() && destination == *subject.CoTargetID
}
