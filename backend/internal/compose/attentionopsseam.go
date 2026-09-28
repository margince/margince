// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The feed's accountability-and-operations seams: the compliance clock and
// the sync's health, each bound to the module that owns what it shows —
// split from attentionseam.go on the one-concept-per-file length cap.

import (
	"context"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attentionDSRs binds the compliance lane to the consent module's own thin
// read; the DSR-admin gate lives in the store, not here.
type attentionDSRs struct{ store *consent.Store }

func (d attentionDSRs) OpenDueSoonest(ctx context.Context, limit int) ([]attention.DSRCase, error) {
	owed, err := d.store.OpenDSRsDueSoonest(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.DSRCase, 0, len(owed))
	for _, request := range owed {
		out = append(out, attention.DSRCase{ID: request.ID, Kind: request.Kind, DueAt: request.DueAt})
	}
	return out, nil
}

// attentionNoticeCases binds contact-linked duties to the consent agenda,
// which requires both compliance authority and access to the linked contact.
type attentionNoticeCases struct{ store *consent.Store }

func (n attentionNoticeCases) OpenDueSoonest(ctx context.Context, limit int, scope attention.TaskScope, owner ids.UUID, team []ids.UUID) ([]attention.NoticeCase, error) {
	in := consent.NoticeAgendaInput{Limit: limit, TeamOwners: team}
	switch scope {
	case attention.TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			return nil, apperrors.ErrPermissionDenied
		}
		in.OwnerID = &actor.UserID
	case attention.TasksOwnedBy:
		if owner.IsZero() {
			return nil, apperrors.ErrPermissionDenied
		}
		in.OwnerID = &owner
	case attention.TasksUnassigned:
		unassigned := ids.Nil
		in.OwnerID = &unassigned
	}
	owed, err := n.store.OpenNoticeCasesDueSoonest(ctx, in)
	if err != nil {
		return nil, err
	}
	out := make([]attention.NoticeCase, 0, len(owed))
	for _, duty := range owed {
		// duty.Blocked is deliberately dropped: a blocked case reaches the lane
		// like any other, and the obstacle is read on the contact's own screen.
		out = append(out, attention.NoticeCase{
			ID: duty.ID, Rule: string(duty.Rule), OwnerID: duty.OwnerID,
			ContactID: duty.ContactID.UUID, DueAt: duty.DueAt,
		})
	}
	return out, nil
}

// captureHealthRegistry composes the registry a health lane reads through:
// bare, with no sink, authority or vault, because
// Registry.HealthConcerns stays within what Connections itself reads and
// anything deeper would be a nil dereference on a request path.
func captureHealthRegistry(db *database.DB) *capture.Registry {
	return capture.NewRegistry(db, nil, nil, nil)
}

// attentionCaptureHealth binds the capture-health lane to the capture
// module's own per-user read; the human-only arm lives there.
type attentionCaptureHealth struct{ registry *capture.Registry }

func (c attentionCaptureHealth) CaptureConcerns(ctx context.Context) ([]attention.CaptureConcern, error) {
	concerns, err := c.registry.HealthConcerns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]attention.CaptureConcern, 0, len(concerns))
	for _, concern := range concerns {
		out = append(out, attention.CaptureConcern{
			ConnectionID: concern.ConnectionID,
			Kind:         concern.Kind,
			Provider:     concern.Provider,
			AccountLabel: concern.AccountLabel,
			FailingSince: concern.FailingSince,
		})
	}
	return out, nil
}

// attentionAIWork binds the AI-work-health lane to the same projection the
// activity rail reads; the contact-only refusal lives in the store's read.
type attentionAIWork struct{ store *aiactivity.Store }

func (a attentionAIWork) Troubled(ctx context.Context, since time.Time, limit int) ([]attention.TroubledRun, error) {
	troubled, err := a.store.Troubled(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.TroubledRun, 0, len(troubled))
	for _, run := range troubled {
		item := attention.TroubledRun{
			ID:         run.ID,
			Kind:       run.Kind,
			State:      run.State,
			OccurredAt: run.OccurredAt,
		}
		if run.Summary != nil {
			item.Summary = *run.Summary
		}
		if run.SubjectLabel != nil {
			item.SubjectLabel = *run.SubjectLabel
		}
		out = append(out, item)
	}
	return out, nil
}

// attentionBounces binds the bounce lane to the comms store's own per-user
// read of the stamp RecordBounce leaves; the contact-only refusal lives there.
type attentionBounces struct{ store *comms.Store }

func (b attentionBounces) HardBounces(ctx context.Context, since time.Time, limit int) ([]attention.BouncedSend, error) {
	bounced, err := b.store.HardBouncesFor(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.BouncedSend, 0, len(bounced))
	for _, send := range bounced {
		out = append(out, attention.BouncedSend{
			ID:        send.ID,
			Subject:   send.Subject,
			Reason:    send.Reason,
			BouncedAt: send.BouncedAt,
			ContactID: send.ContactID,
			Recipient: send.Recipient,
		})
	}
	return out, nil
}

// attentionUndelivered binds the undelivered lane to the comms store's own
// per-user read of the stamp the dispatcher's park leaves; the contact-only
// refusal lives there.
type attentionUndelivered struct{ store *comms.Store }

func (u attentionUndelivered) ParkedSends(ctx context.Context, since time.Time, limit int) ([]attention.ParkedSend, error) {
	parked, err := u.store.ParkedSendsFor(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.ParkedSend, 0, len(parked))
	for _, send := range parked {
		out = append(out, attention.ParkedSend{
			ID:        send.ID,
			Subject:   send.Subject,
			Reason:    send.Reason,
			ParkedAt:  send.ParkedAt,
			ContactID: send.ContactID,
		})
	}
	return out, nil
}

// attentionAutomations binds the rule-health lane to the automation store's
// own cross-instance read; the automation-read gate lives there.
type attentionAutomations struct{ store *automation.AutomationStore }

func (a attentionAutomations) TroubledRuns(ctx context.Context, since time.Time, limit int) ([]attention.TroubledAutomationRun, error) {
	troubled, err := a.store.TroubledRuns(ctx, since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.TroubledAutomationRun, 0, len(troubled))
	for _, run := range troubled {
		item := attention.TroubledAutomationRun{
			ID:           run.ID,
			AutomationID: run.AutomationID,
			Name:         run.Name,
			Outcome:      run.Outcome,
			OccurredAt:   run.CreatedAt,
		}
		if run.Reason != nil {
			item.Reason = *run.Reason
		}
		out = append(out, item)
	}
	return out, nil
}

// attentionNotices binds the notices lane to the store's own per-user read.
type attentionNotices struct {
	store *notices.Store
	users *identity.Service
}

func (a attentionNotices) Unread(ctx context.Context, limit int) ([]attention.UnreadNotice, error) {
	unread, err := a.store.UnreadFor(ctx, limit)
	if err != nil {
		return nil, err
	}
	seats := []ids.UserID{}
	for _, notice := range unread {
		if notice.Origin != nil && notice.Origin.OnBehalfOf != nil {
			seats = append(seats, ids.From[ids.UserKind](ids.UUID(*notice.Origin.OnBehalfOf)))
		}
		if notice.Origin != nil && notice.Origin.ActorType == string(principal.PrincipalHuman) {
			if id, parseErr := ids.Parse(strings.TrimPrefix(notice.Origin.ActorId, "human:")); parseErr == nil {
				seats = append(seats, ids.From[ids.UserKind](id))
			}
		}
	}
	names, err := a.users.SeatNames(ctx, seats)
	if err != nil {
		return nil, err
	}
	out := make([]attention.UnreadNotice, 0, len(unread))
	for _, notice := range unread {
		if notice.Origin != nil && notice.Origin.ActorType == string(principal.PrincipalHuman) {
			if id, parseErr := ids.Parse(strings.TrimPrefix(notice.Origin.ActorId, "human:")); parseErr == nil {
				if name := names[id]; name != "" {
					notice.Origin.ActorName = &name
				}
			}
		}
		if notice.Origin != nil && notice.Origin.OnBehalfOf != nil {
			if name := names[ids.UUID(*notice.Origin.OnBehalfOf)]; name != "" {
				notice.Origin.OnBehalfOfName = &name
			}
		}
		out = append(out, attention.UnreadNotice{
			ID:         notice.ID,
			Origin:     notice.Origin,
			Kind:       notice.Kind,
			Subject:    notice.Subject,
			Body:       notice.Body,
			TargetType: notice.Target.Type,
			TargetID:   notice.Target.ID,
			CreatedAt:  notice.CreatedAt,
		})
	}
	return out, nil
}

// attentionIntroductions binds the introductions lane to the store's own
// per-user read.
//
// Per-user for the reason attentionNotices is: an ask names one colleague, so
// the store refuses a caller with no human behind it and the lane renders that
// as withheld. Nothing here widens, and there is no scope argument to pass —
// the read takes none.
type attentionIntroductions struct{ store *introductions.Store }

func (a attentionIntroductions) Pending(
	ctx context.Context, limit int,
) ([]attention.PendingIntroduction, error) {
	asks, err := a.store.AwaitingMyAnswer(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]attention.PendingIntroduction, 0, len(asks))
	for _, ask := range asks {
		out = append(out, attention.PendingIntroduction{
			ID:        ask.ID,
			ContactID: ask.ContactID,
			// The requester's own words, carried rather than summarised: the
			// colleague is deciding whether to spend their relationship, and a
			// paraphrase is not what they would be agreeing to.
			Reason:      ask.InternalReason,
			RequestedAt: ask.RequestedAt,
			DueAt:       ask.DueAt,
		})
	}
	return out, nil
}
