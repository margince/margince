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
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func (r approvalPreviewReader) scheduledApprovalPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	raw, err := json.Marshal(approval.ProposedChange)
	if err != nil {
		return nil, err
	}
	message, sender, err := r.heldApprovalMessage(ctx, raw)
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
		return out, nil //nolint:nilerr // Invalid persisted mail input leaves the held decision incomplete.
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
	scheduledDecisionDetails(out)
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

func (r approvalPreviewReader) heldApprovalMessage(ctx context.Context, raw json.RawMessage) (activities.ScheduledSend, string, error) {
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

func scheduledDecisionDetails(out *crmcontracts.ApprovalPreview) {
	out.Effects = &[]string{"rearm_held_message"}
	*out.Details = append(*out.Details, previewDetail("retry_delay_minutes", strconv.FormatFloat(heldRetryDelay.Minutes(), 'f', -1, 64)))
}
