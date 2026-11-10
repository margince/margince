// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type InquiryPreview struct {
	SourceID      ids.UUID
	SourceVersion int64
	Target        datasource.EntityRef
	Recipient     ids.UUID
	Reason        string
	DueAt         time.Time
	Effect        workflow.Effect
}

func InquiryDueAt(policy crmcontracts.WebsiteInquiryPolicy, received time.Time) (time.Time, error) {
	raw, err := json.Marshal(policy)
	if err != nil {
		return time.Time{}, err
	}
	return taskDueAt(raw, received, 1)
}

func (s *InquiryService) Preview(ctx context.Context, ex Executors, ev workflow.Event) (InquiryPreview, error) {
	policy, err := ParseInquiryPolicy(ev.Params)
	if err != nil {
		return InquiryPreview{}, err
	}
	receipt, err := readAcceptedInquiry(ctx, ex, ev, policy, true)
	if err != nil {
		return InquiryPreview{}, err
	}
	admit, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return InquiryPreview{}, err
	}
	recipient, reason, err := s.routing.Preview(ctx, admit, policy, receipt.Target)
	if err != nil {
		return InquiryPreview{}, err
	}
	if err := s.routing.CheckRecipient(ctx, recipient, receipt.Target, ev.Entity.ID); err != nil {
		return InquiryPreview{}, err
	}
	due, err := InquiryDueAt(policy, receipt.Activity.CreatedAt)
	if err != nil {
		return InquiryPreview{}, err
	}
	effect, err := planInquiryTask(ctx, ev, policy, receipt)
	if err != nil {
		return InquiryPreview{}, err
	}
	var args map[string]json.RawMessage
	if err := json.Unmarshal(effect.Actions[0].Args, &args); err != nil {
		return InquiryPreview{}, err
	}
	args["assignee_id"], err = json.Marshal(recipient)
	if err != nil {
		return InquiryPreview{}, err
	}
	effect.Actions[0].Args, err = json.Marshal(args)
	if err != nil {
		return InquiryPreview{}, err
	}
	if receipt.Activity.Version == nil {
		return InquiryPreview{}, errors.New("automation: the accepted inquiry has no version")
	}
	return InquiryPreview{SourceID: ev.Entity.ID, SourceVersion: int64(*receipt.Activity.Version), Target: receipt.Target, Recipient: recipient, Reason: reason, DueAt: due, Effect: effect}, nil
}
