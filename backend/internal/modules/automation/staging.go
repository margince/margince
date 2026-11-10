// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func stageForApproval(ctx context.Context, approvals Approvals, language baselanguage.Resolver, action workflow.Action) (ids.ApprovalID, error) {
	raw := action.Args
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	summary := fmt.Sprintf(summaryIn(ctx, language).stagedAction, action.Kind, action.Target.Type, action.Target.ID)
	metadata := systemtext.Sentence("automation.stagedAction", systemtext.ActionKindArgument(string(action.Kind)), systemtext.NounArgument(string(action.Target.Type)), systemtext.Text(action.Target.ID.String()))
	if action.Kind == workflow.ActionEmitFlowEvent {
		request, err := ReadConfirmationRequest(raw)
		if err != nil {
			return ids.ApprovalID{}, err
		}
		summary = fmt.Sprintf(summaryIn(ctx, language).confirmation, request.Question)
		metadata = systemtext.Sentence("automation.confirmation", systemtext.Text(request.Question))
	}
	canonical, diffHash, err := diffhash.Canonical(raw)
	if err != nil {
		return ids.ApprovalID{}, fmt.Errorf("automation: canonicalizing %s for staging: %w", action.Kind, err)
	}
	return approvals.Stage(ctx, StageRequest{
		Kind:           string(action.Kind),
		ProposedChange: canonical,
		DiffHash:       diffHash,
		TargetType:     string(action.Target.Type),
		TargetID:       action.Target.ID,
		Summary:        summary,
		SystemText:     &systemtext.Message{Subject: metadata},
	})
}

// HeldDraftKind keeps paused rule drafts on the existing approval queue.
const HeldDraftKind = "held_draft"

func stageHeldDraft(ctx context.Context, approvals Approvals, language baselanguage.Resolver, target datasource.EntityRef, proposal HeldDraftProposal) (ids.ApprovalID, error) {
	if approvals == nil {
		return ids.ApprovalID{}, ErrNoApprovalStaging
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		return ids.ApprovalID{}, fmt.Errorf("automation: encoding a held draft for staging: %w", err)
	}
	canonical, diffHash, err := diffhash.Canonical(raw)
	if err != nil {
		return ids.ApprovalID{}, fmt.Errorf("automation: canonicalizing a held draft for staging: %w", err)
	}
	return approvals.Stage(ctx, StageRequest{
		Kind:           HeldDraftKind,
		ProposedChange: canonical,
		DiffHash:       diffHash,
		TargetType:     string(target.Type),
		TargetID:       target.ID,
		Summary:        fmt.Sprintf(summaryIn(ctx, language).heldDraft, proposal.To),
		SystemText:     &systemtext.Message{Subject: systemtext.Sentence("automation.heldDraft", systemtext.Label(proposal.To))},
		JoinPending:    true,
	})
}
