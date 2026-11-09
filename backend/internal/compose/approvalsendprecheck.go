// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func stagedSendApproval(in approvals.StageInput) (crmcontracts.Approval, error) {
	var payload map[string]any
	if err := json.Unmarshal(in.ProposedChange, &payload); err != nil {
		return crmcontracts.Approval{}, fmt.Errorf("read staged send: %w", err)
	}
	return crmcontracts.Approval{
		Kind: in.Kind, ProposedChange: &payload,
		TargetEntityType: new(in.TargetType), TargetEntityId: new(crmcontracts.Id(in.TargetID)),
	}, nil
}

func agentMailPrecheck(store *activities.Store, gate *consent.Gate) approvals.StagePrecheck {
	return func(ctx context.Context, in approvals.StageInput) error {
		approval, err := stagedSendApproval(in)
		if err != nil {
			return err
		}
		request, err := commercialRequest[crmcontracts.SendCompanyEmailRequest](approval)
		if err != nil {
			return err
		}
		message, err := activities.DecodeCompanyEmailRequest(request)
		if err != nil {
			return err
		}
		origin, _, err := mailApprovalOrigin(ctx, store, approval, request)
		if err != nil {
			return err
		}
		decisions, err := store.PreviewSendExecution(ctx, origin, message, gate)
		if err != nil {
			return err
		}
		return refuseAtStaging(decisions)
	}
}

func agentChannelPrecheck(store *activities.Store, gate *consent.Gate) approvals.StagePrecheck {
	return func(ctx context.Context, in approvals.StageInput) error {
		approval, err := stagedSendApproval(in)
		if err != nil {
			return err
		}
		_, message, err := channelMessageRequest(approval)
		if err != nil {
			return err
		}
		request, err := store.PreviewChannelSend(ctx, ids.From[ids.ActivityKind](in.TargetID), message)
		if err != nil {
			return err
		}
		decisions, err := gate.Preview(ctx, request)
		if err != nil {
			return err
		}
		return refuseAtStaging(decisions)
	}
}
