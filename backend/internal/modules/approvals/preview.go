// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/approvalplan"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PreviewContextError distinguishes missing review context from changed records.
type PreviewContextError struct{ State string }

func (e *PreviewContextError) Error() string {
	return "approval context is " + e.State
}

// MessageFault gives clients a recoverable context refusal without internal details.
func (e *PreviewContextError) MessageFault() (code, message string) {
	switch e.State {
	case previewIncomplete:
		return "approval_preview_incomplete", "The full change cannot be shown. Resolve the missing context before approving."
	case "conflict":
		return "approval_preview_conflict", "Resolve the conflicting context before approving this change."
	case PreviewNeedsWiderSight:
		return "approval_needs_wider_sight", "This change touches records you cannot open. A colleague who can see them must decide it."
	default:
		return "approval_preview_unavailable", "The context needed to review this change is unavailable. Check access and the source before approving."
	}
}

// Unwrap makes a refusal for want of sight a 403: who decides is the problem,
// not the context.
func (e *PreviewContextError) Unwrap() error {
	if e.State == PreviewNeedsWiderSight {
		return apperrors.ErrPermissionDenied
	}
	return nil
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
	preview, _, err := s.proposalPreview(ctx, a, proposal)
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
	if (preview.Version != 1 && preview.Version != 2) || preview.State != "ready" {
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
		preview, heldBack, err := s.proposalPreview(ctx, subjects[i], rows[i])
		if err != nil {
			return err
		}
		rows[i].Preview = preview
		if (approvalplan.PreviewRead{Preview: preview, BuiltAsAgent: s.runsAsAgent(stagedByRow(subjects[i])), HeldBack: heldBack}).PayloadWithheld() {
			// The staged call and its summary say what the preview held back.
			rows[i].ProposedChange, rows[i].Evidence, rows[i].Summary = nil, nil, nil
		}
	}
	return nil
}

// proposalPreview also answers whether the build masked a value for the reader.
func (s *Service) proposalPreview(ctx context.Context, subject row, proposal crmcontracts.Approval) (preview *crmcontracts.ApprovalPreview, heldBack bool, err error) {
	recording, masked := withMaskRecorder(s.withPreviewEffect(ctx, subject))
	preview, err = s.preview(recording, proposal)
	if err != nil || preview == nil {
		return preview, false, err
	}
	heldBack = masked.Load()
	defer func() {
		if preview.State != "ready" {
			preview.Changes = []crmcontracts.ApprovalPreviewChange{}
			preview.Effects = nil
			preview.Details = nil
			preview.Files = nil
		}
	}()
	if !previewCoTargetPinned(subject, proposal) {
		preview.State = previewIncomplete
		return preview, heldBack, nil
	}
	if _, contextPinned := s.contextPins[subject.Kind]; subject.CoTargetVersion == nil && !contextPinned {
		return preview, heldBack, nil
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := checkPin(ctx, tx, subject.CoTargetType, subject.CoTargetID, subject.CoTargetVersion, "second record"); err != nil {
			return err
		}
		return s.previewContextPin(ctx, tx, subject)
	})
	if errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) {
		preview.State = "stale"
		return preview, heldBack, nil
	}
	return preview, heldBack, err
}

func previewCoTargetPinned(subject row, proposal crmcontracts.Approval) bool {
	if subject.Kind == previewMergeRecords && subject.TargetType != nil && (*subject.TargetType == "contact" || *subject.TargetType == tableCompany) {
		return previewMergePinsMatch(subject, proposal)
	}
	var entityType, field string
	switch subject.Kind {
	case "advance_deal", "progress_deal", previewStageProgression:
		entityType, field = "stage", "to_stage_id"
	case "merge_tags":
		entityType, field = "tag", "into_tag_id"
	default:
		return true
	}
	if subject.CoTargetVersion == nil || subject.CoTargetType == nil || *subject.CoTargetType != entityType || subject.CoTargetID == nil || proposal.ProposedChange == nil {
		return false
	}
	return proposalDestination(proposal, field) == *subject.CoTargetID && !subject.CoTargetID.IsZero()
}

func proposalDestination(proposal crmcontracts.Approval, field string) ids.UUID {
	raw, err := json.Marshal(proposal.ProposedChange)
	if err != nil {
		return ids.Nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ids.Nil
	}
	if _, rest := payload["operation"]; rest {
		body := payload["body"]
		payload = nil
		if err := json.Unmarshal(body, &payload); err != nil {
			return ids.Nil
		}
	}
	var destination ids.UUID
	if err := json.Unmarshal(payload[field], &destination); err != nil {
		return ids.Nil
	}
	return destination
}

// PreviewNeedsWiderSight is the state of a preview whose effect touches a
// record the viewer cannot open. It shows nothing of the plan.
const PreviewNeedsWiderSight = approvalplan.NeedsWiderSight

const (
	previewIncomplete       = "incomplete"
	previewStageProgression = "stage_progression"
)

const previewMergeRecords = "merge_records"
