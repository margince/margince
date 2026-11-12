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
	ExistingTask  *crmcontracts.Activity
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
	if task, found, err := s.existingTask(ctx, ex, ev, receipt); err != nil {
		return InquiryPreview{}, err
	} else if found {
		if receipt.Activity.Version == nil {
			return InquiryPreview{}, errors.New("automation: the accepted inquiry has no version")
		}
		return InquiryPreview{SourceID: ev.Entity.ID, SourceVersion: int64(*receipt.Activity.Version), Target: receipt.Target, ExistingTask: &task}, nil
	}
	admit, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return InquiryPreview{}, err
	}
	if frozen, found, err := s.frozenPreview(ctx, ev, receipt); found || err != nil {
		if err != nil {
			return frozen, err
		}
		if err := s.routing.CheckRecipient(admit, frozen.Recipient, frozen.Target, frozen.SourceID); err != nil {
			return frozen, err
		}
		return frozen, nil
	}
	recipient, reason, err := s.routing.Preview(ctx, admit, policy, receipt.Target, ev.Entity.ID)
	if err != nil {
		return InquiryPreview{}, err
	}
	if err := s.routing.CheckRecipient(admit, recipient, receipt.Target, ev.Entity.ID); err != nil {
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

func (s *InquiryService) frozenPreview(ctx context.Context, ev workflow.Event, receipt inquiryReceipt) (InquiryPreview, bool, error) {
	plan, found, err := s.claims.InquiryPlan(ctx, receipt.Key)
	if err != nil || !found {
		return InquiryPreview{}, found, err
	}
	if plan.PolicyID != ev.AutomationID || plan.SourceActivityID != ev.Entity.ID || plan.Target != receipt.Target {
		return InquiryPreview{}, true, workflow.Declined("the saved inquiry assignment no longer matches its source")
	}
	if receipt.Activity.Version == nil {
		return InquiryPreview{}, true, errors.New("automation: the accepted inquiry has no version")
	}
	action, err := s.originalTask(ctx, plan)
	if err != nil {
		return InquiryPreview{}, true, err
	}
	return InquiryPreview{SourceID: ev.Entity.ID, SourceVersion: int64(*receipt.Activity.Version), Target: plan.Target, Recipient: plan.Recipient, Reason: plan.Reason, DueAt: plan.DueAt, Effect: workflow.Effect{Actions: []workflow.Action{action}}}, true, nil
}
