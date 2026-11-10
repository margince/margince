// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type websiteInquiry struct{ ex Executors }

func (websiteInquiry) Spec() workflow.Spec {
	return workflow.Spec{Name: inquiryRecipeName, Trigger: workflow.Trigger{EventType: eventActivityCaptured}, Tier: mcp.TierAutoExecute, RedrivableWithoutDuplicating: true}
}

func (websiteInquiry) Match(_ context.Context, ev workflow.Event) (bool, error) {
	policy, err := ParseInquiryPolicy(ev.Params)
	if err != nil {
		return false, err
	}
	var payload crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return false, err
	}
	if payload.SourceSystem == nil || *payload.SourceSystem != provenance.WebFormSource || payload.WebsiteSubmissionKey == nil {
		return false, nil
	}
	form, _, ok := strings.Cut(*payload.WebsiteSubmissionKey, ":")
	return ok && form == policy.FormId.String() && payload.Kind == "note", nil
}

func (w websiteInquiry) Plan(ctx context.Context, ev workflow.Event) (workflow.Effect, error) {
	policy, err := ParseInquiryPolicy(ev.Params)
	if err != nil {
		return workflow.Effect{}, err
	}
	receipt, err := readInquiryReceipt(ctx, w.ex, ev, policy)
	if err != nil {
		return workflow.Effect{}, err
	}
	return planInquiryTask(ctx, ev, policy, receipt)
}

func planInquiryTask(ctx context.Context, ev workflow.Event, policy crmcontracts.WebsiteInquiryPolicy, receipt inquiryReceipt) (workflow.Effect, error) {
	due, err := InquiryDueAt(policy, receipt.Activity.CreatedAt)
	if err != nil {
		return workflow.Effect{}, err
	}
	targetEvent := ev
	targetEvent.Entity = receipt.Target
	key := taskNaturalKey{System: provenance.WebsiteInquirySource, ID: receipt.Key}
	if receipt.Target.Type == datasource.EntityContact {
		key.System, key.ID = provenance.LeadForContactSource, provenance.WebFormSource+":"+receipt.Key
	}
	effect, err := taskCreateEffectFor(ctx, targetEvent, policy.TaskTitle, due, nil, &key)
	if err != nil {
		return workflow.Effect{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(effect.Actions[0].Args, &fields); err != nil {
		return workflow.Effect{}, err
	}
	fields["source_activity_id"], err = json.Marshal(ev.Entity.ID)
	if err != nil {
		return workflow.Effect{}, err
	}
	effect.Actions[0].Args, err = json.Marshal(fields)
	return effect, err
}

func (w websiteInquiry) Apply(ctx context.Context, ev workflow.Event, effect workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	if w.ex.Inquiries == nil {
		return workflow.RunResult{}, errors.New("automation: inquiry handling is not configured")
	}
	return w.ex.Inquiries.Apply(ctx, w.ex, ev, effect)
}

func (websiteInquiry) IdempotencyKey(ev workflow.Event) string {
	var payload crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(ev.Payload, &payload); err != nil || payload.WebsiteSubmissionKey == nil {
		return inquiryRecipeName + ":" + ev.ID.String()
	}
	return inquiryRecipeName + ":" + *payload.WebsiteSubmissionKey
}
