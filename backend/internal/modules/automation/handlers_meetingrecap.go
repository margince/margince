// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type postMeetingRecap struct{ ex Executors }

const (
	postMeetingRecapName = "post_meeting_recap"
	recapIntent          = "post-meeting recap"
)

// The fixed recap template carries its purpose alongside the communication context.
// Archived inbound anchors can require the purpose fallback during send checks.
const purposeBusinessCorrespondence = "business_correspondence"

func (postMeetingRecap) Spec() workflow.Spec {
	return workflow.Spec{
		Name:                  postMeetingRecapName,
		Trigger:               workflow.Trigger{EventType: eventActivityCaptured, AdditionalEventTypes: []string{eventActivityUpdated}},
		Tier:                  mcp.TierAutoExecute,
		IgnoreUnmatchedEvents: true,
	}
}

func (w postMeetingRecap) Match(ctx context.Context, ev workflow.Event) (bool, error) {
	if ev.Entity.Type != datasource.EntityActivity ||
		events.IsImported(events.Envelope{Type: ev.Type, Payload: ev.Payload}) {
		return false, nil
	}
	// Existing enabled rules keep their trigger until enabled again.
	if ev.EnabledFrom.IsZero() {
		if ev.Type != eventActivityCaptured {
			return false, nil
		}
		return meetingRecapCandidate(ev)
	}
	if ev.OwnerID == ids.Nil || ev.OccurredAt.Before(ev.EnabledFrom) {
		return false, nil
	}
	candidate, err := meetingRecapCandidate(ev)
	if err != nil || !candidate {
		return false, err
	}
	if w.ex.MeetingRecaps == nil {
		return false, errors.New("automation: meeting recap reader is not configured")
	}
	owner, err := ownerContext(ctx, w.ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return false, err
	}
	ready, err := w.ex.MeetingRecaps.ReadyForRecap(owner, ids.From[ids.ActivityKind](ev.Entity.ID), ev.OccurredAt)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return ready, err
}

func meetingRecapCandidate(ev workflow.Event) (bool, error) {
	switch ev.Type {
	case eventActivityCaptured:
		var captured crmcontracts.PublicEventActivityCaptured
		if err := json.Unmarshal(ev.Payload, &captured); err != nil {
			return false, fmt.Errorf("automation: decoding meeting capture: %w", err)
		}
		return captured.Kind == string(crmcontracts.ActivityKindMeeting), nil
	case eventActivityUpdated:
		var updated crmcontracts.PublicEventActivityUpdated
		if err := json.Unmarshal(ev.Payload, &updated); err != nil {
			return false, fmt.Errorf("automation: decoding meeting update: %w", err)
		}
		return updated.ChangedFields.MeetingRecapBecameReady != nil && *updated.ChangedFields.MeetingRecapBecameReady, nil
	default:
		return false, nil
	}
}

func (postMeetingRecap) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	// This starter fixes its purpose; user-configured draft templates must supply their own.
	args, err := json.Marshal(draftEmailArgs{
		Intent:               recapIntent,
		ConsentPurpose:       purposeBusinessCorrespondence,
		CommunicationContext: commsauthz.CategoryReplyToInbound,
	})
	if err != nil {
		return workflow.Effect{}, fmt.Errorf("automation: encoding the draft_email action: %w", err)
	}
	// Target is the captured meeting activity itself: applyDraftEmail anchors
	// the composed draft on it (Comms.DraftEmail's anchor), and the draft
	// lands durably on workflow_run.applied.
	return workflow.Effect{Actions: []workflow.Action{{
		Kind: workflow.ActionDraftEmail, Target: ev.Entity, Args: args,
	}}}, nil
}

func (w postMeetingRecap) Apply(ctx context.Context, ev workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	ex := w.ex
	if !ev.EnabledFrom.IsZero() {
		if ex.RecapDraftClaims == nil {
			return workflow.RunResult{}, ErrNoEffectClaims
		}
		if ex.Approvals == nil {
			return workflow.RunResult{}, ErrNoApprovalStaging
		}
		ex.Approvals = meetingRecapApprovals{ex.Approvals, ex.RecapDraftClaims, w.IdempotencyKey(ev)}
	}
	applied, err := ApplyActions(ctx, ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (postMeetingRecap) IdempotencyKey(ev workflow.Event) string {
	if ev.EnabledFrom.IsZero() {
		return postMeetingRecapName + ":" + ev.ID.String()
	}
	return postMeetingRecapName + ":" + ev.Entity.ID.String()
}

type meetingRecapApprovals struct {
	approvals  Approvals
	claims     RecapDraftClaims
	occurrence string
}

func (a meetingRecapApprovals) Stage(ctx context.Context, in StageRequest) (ids.ApprovalID, error) {
	return a.claims.StageOnce(ctx, postMeetingRecapName, a.occurrence, HeldDraftKind, func(bound context.Context) (ids.ApprovalID, error) {
		return a.approvals.Stage(bound, in)
	})
}
