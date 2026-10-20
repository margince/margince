// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (r approvalPreviewReader) scheduledApprovalMessage(ctx context.Context, approval crmcontracts.Approval) (activities.ScheduledSend, string, error) {
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return activities.ScheduledSend{}, "", err
	}
	if approval.Kind == heldScheduledSendKind {
		id, err := heldMessageID(raw)
		if err != nil {
			return activities.ScheduledSend{}, "", apperrors.ErrInvalidArgument
		}
		message, err := r.activities.GetScheduledSend(ctx, id)
		if err != nil {
			return activities.ScheduledSend{}, "", err
		}
		var staged heldProposal
		if json.Unmarshal(raw, &staged) != nil || staged.Reason != message.HeldReason {
			return activities.ScheduledSend{}, "", apperrors.ErrVersionSkew
		}
		sender := message.PrincipalKind + ":" + message.ScheduledBy.String()
		return message, sender, nil
	}
	decision, err := decodeReviewDecision(raw)
	if err != nil {
		return activities.ScheduledSend{}, "", apperrors.ErrInvalidArgument
	}
	id, err := ids.Parse(decision.ReviewID)
	if err != nil {
		return activities.ScheduledSend{}, "", apperrors.ErrInvalidArgument
	}
	review, err := consent.NewStore(r.db).ReviewForReader(ctx, id)
	if err != nil {
		return activities.ScheduledSend{}, "", err
	}
	if review.State != consent.ReviewAwaitingDecision || review.ApprovalID != ids.UUID(approval.Id) || review.IntentID.String() != decision.IntentID {
		return activities.ScheduledSend{}, "", apperrors.ErrVersionSkew
	}
	message, err := r.activities.ReadScheduledSendForDecision(ctx, review.IntentID, review.InitiatedBy)
	if err != nil {
		return activities.ScheduledSend{}, "", err
	}
	if message.HeldReason != activities.HeldSendRefused {
		return activities.ScheduledSend{}, "", apperrors.ErrVersionSkew
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return activities.ScheduledSend{}, "", apperrors.ErrPermissionDenied
	}
	return message, "human:" + actor.UserID.String(), nil
}

func (r approvalPreviewReader) scheduledApprovalPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	message, sender, err := r.scheduledApprovalMessage(ctx, approval)
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
	if message.Status != activities.ScheduledStatusHeld {
		out.State = previewStaleState
		return out, nil
	}
	input, additional, err := message.PreviewMessage()
	if err != nil {
		return out, nil
	}
	links := message.Links
	if !message.Anchor.IsZero() {
		links, err = activities.FromActivity(message.Anchor).Resolve(ctx, r.activities)
		if previewUnavailable(err) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
	links = append(links, additional...)
	if err := r.populateFrozenMessage(ctx, input, links, out); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	effect := "rearm_held_message"
	if approval.Kind == approvals.KindCommunicationReview {
		effect = "direct_refused_message"
	}
	out.Effects = &[]string{effect}
	if approval.Kind == heldScheduledSendKind {
		*out.Details = append(*out.Details, previewDetail("retry_delay_minutes", strconv.FormatFloat(heldRetryDelay.Minutes(), 'f', -1, 64)))
	} else {
		decision, err := commercialRequest[reviewDecision](approval)
		if err != nil {
			return out, nil
		}
		*out.Details = append(*out.Details, previewDetail("operator_reason", decision.recordedExplanation()), previewDetail("promotion_restriction", decision.ReasonCode))
	}
	senderApproval := approval
	senderApproval.ProposedBy = sender
	if err := r.appendMailPresentation(ctx, senderApproval, input, out); err != nil {
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	out.State = previewReadyState
	return out, nil
}
