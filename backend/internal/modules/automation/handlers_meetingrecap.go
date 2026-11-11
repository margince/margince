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

const postMeetingRecapName = "post_meeting_recap"
const recapIntent = "post-meeting recap"

// purposeBusinessCorrespondence is the seeded consent purpose whose class is
// never consent-gated (ADR-0098 D1): answering somebody individually rests on
// Art 6(1)(b)/(f), and consent with its German evidence standard belongs to
// marketing. Spelled once here so a starter and its tests cannot disagree
// about which purpose a recap is sent under.
//
// It rides ALONGSIDE the context rather than being replaced by it. The context
// is what the engine decides on; the purpose is what the legacy fallback reads
// when a record supports no category on its own evidence. A recap into a thread
// with a real inbound anchor never reaches that fallback, but one whose anchor
// has since been archived does, and dropping the key would turn that into a
// denial rather than the answer the old model gave.
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
	ready, err := w.ex.MeetingRecaps.ReadyForRecap(owner, ids.From[ids.ActivityKind](ev.Entity.ID), ev.OccurredAt, ev.EnabledFrom)
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
		return updated.ChangedFields.Body != nil || updated.ChangedFields.MeetingStatus != nil, nil
	default:
		return false, nil
	}
}

func (postMeetingRecap) Plan(_ context.Context, ev workflow.Event) (workflow.Effect, error) {
	// business_correspondence is NAMED here rather than left to a default, and
	// naming it is not this code choosing a lawful basis: the purpose exists
	// for exactly this case. ADR-0098 D1 added it because ADR-0011's blanket
	// default-deny "overshoots on one class: replying to a contact who wrote to
	// us" — individual business correspondence is not advertising under UWG §7
	// and rests on Art 6(1)(b)/(f), not consent. A recap to the contact you just
	// met with is that and nothing else.
	//
	// A user-configured draft_email instance declares its own purpose and is
	// refused without one (MissingConsentPurposeError). This is a seeded
	// starter whose recipient and register are both fixed by the template, so
	// its purpose is fixed with them.
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

func (w postMeetingRecap) Apply(ctx context.Context, _ workflow.Event, eff workflow.Effect, _ *workflow.ApprovalToken) (workflow.RunResult, error) {
	applied, err := ApplyActions(ctx, w.ex, eff)
	return workflow.RunResult{Applied: applied}, err
}

func (postMeetingRecap) IdempotencyKey(ev workflow.Event) string {
	if ev.EnabledFrom.IsZero() {
		return postMeetingRecapName + ":" + ev.ID.String()
	}
	return postMeetingRecapName + ":" + ev.Entity.ID.String()
}
