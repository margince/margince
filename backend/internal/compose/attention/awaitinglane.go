// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The follow-up reminder: a message the reader sent to a customer that nobody
// has answered once the workspace's follow-up window has passed. The mirror of
// the who-is-waiting lane, read beside the day the way that one is.

import (
	"context"
	"errors"
	"log/slog"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	sourceAwaitingReply   = "awaiting_reply"
	sourceMeetingFollowUp = "meeting_follow_up"
)

// Awaiting reads the messages the reader sent that nobody has answered.
type Awaiting interface {
	// cut reports that the read stopped at its own bound.
	AwaitingReplies(ctx context.Context, asOf time.Time) (rows []AwaitedReply, cut bool, err error)
	// MeetingFollowUps are the reader's meetings with a customer they have
	// sent nothing to since; SentAt is when the meeting started.
	MeetingFollowUps(ctx context.Context, asOf time.Time) (rows []AwaitedReply, cut bool, err error)
}

// AwaitedReply is one message the reader sent and is still waiting on.
type AwaitedReply struct {
	ActivityID ids.UUID
	Subject    string
	SentAt     time.Time
	ContactID  ids.UUID
	CompanyID  ids.UUID
	DealID     ids.UUID
	// Present when the reader may read the message, which is what the
	// composer drafts the follow-up from.
	EmailSummary *crmcontracts.EmailSummary
}

// followUpRead is the reader's follow-ups, read per request like planRows:
// read says the source ran, cut that it stopped at its bound.
type followUpRead struct {
	rows        []ranked
	read        bool
	cut         bool
	meetingsCut bool
}

// WithAwaiting binds the follow-up reader. Unbound, the source is absent.
func (s *Service) WithAwaiting(a Awaiting) *Service {
	s.awaiting = a
	return s
}

// readingAwaiting reads the reader's follow-ups onto a per-read copy of the
// service, the way readingPlan carries the plan rows. The rows are the
// reader's own sends: they ride the reader's own day and the wider team and
// all views that include it, never a colleague's queue or the unassigned one.
func (s *Service) readingAwaiting(ctx context.Context, asOf time.Time) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	if s.awaiting == nil || s.taskScope == TasksOwnedBy || s.taskScope == TasksUnassigned {
		return &scoped, nil
	}
	var rows, meetings []AwaitedReply
	var cut, meetingsCut bool
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		if rows, cut, err = s.awaiting.AwaitingReplies(ctx, asOf); err != nil {
			return err
		}
		meetings, meetingsCut, err = s.awaiting.MeetingFollowUps(ctx, asOf)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return &scoped, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceAwaitingReply, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		}
	case err != nil:
		slog.ErrorContext(ctx, "the follow-up read failed", "error", err)
		return &scoped, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceAwaitingReply, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	}
	scoped.followUps = followUpRead{
		rows: make([]ranked, 0, len(rows)+len(meetings)), read: true, cut: cut, meetingsCut: meetingsCut,
	}
	for _, row := range rows {
		scoped.followUps.rows = append(scoped.followUps.rows, classifyAwaiting(row, asOf))
	}
	for _, meeting := range meetings {
		scoped.followUps.rows = append(scoped.followUps.rows, classifyMeetingFollowUp(meeting, asOf))
	}
	return &scoped, nil
}

// classifyAwaiting is one follow-up row. It is the reader's own promise to
// keep the conversation going, so it ranks with promises, below a customer
// who is waiting on them.
func classifyAwaiting(awaited AwaitedReply, asOf time.Time) ranked {
	days := daysSince(awaited.SentAt, asOf)
	row := crmcontracts.WorklistItem{
		Id:          awaited.ActivityID.String(),
		Source:      sourceAwaitingReply,
		Category:    crmcontracts.WorklistItemCategoryTasks,
		Level:       levelPromise,
		Consequence: crmcontracts.WorklistItemConsequenceNone,
		Because: []crmcontracts.WorklistReason{
			reason("you_wrote_last", nil), reason("no_reply_days", daysValue(days)),
		},
		Actions:      []crmcontracts.WorklistItemActions{},
		EmailSummary: awaited.EmailSummary,
		Subject: waitingSubject(WaitingCustomer{
			DealID: awaited.DealID, ContactID: awaited.ContactID, CompanyID: awaited.CompanyID,
		}),
	}
	if awaited.Subject != "" {
		row.Title = &awaited.Subject
	}
	if !awaited.ContactID.IsZero() {
		row.Contact = &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(awaited.ContactID)}
	}
	if openableSubject(row.Subject) {
		row.Actions = append(row.Actions, crmcontracts.WorklistItemActions(actionOpen))
		// The composer drafts from the message, so only a message the reader
		// may read offers it.
		if row.EmailSummary != nil {
			row.Actions = append(row.Actions, crmcontracts.WorklistItemActionsReply)
		}
	}
	sent := awaited.SentAt
	row.OccurredAt = &sent
	anchor := openapi_types.UUID(awaited.ActivityID)
	row.Move = &crmcontracts.WorklistMove{Action: crmcontracts.WorklistMoveActionDraftReply, ActivityId: &anchor}
	return ranked{
		item: row, waitingDays: days, waitingRank: orderingAge(days), occurredAt: sent,
		ownerRef: ownedByWhoeverIsReading(), contact: awaited.ContactID,
	}
}

// classifyMeetingFollowUp is one meeting the reader owes a follow-up. The
// composer writes a fresh message to the contact rather than a reply, since a
// meeting is not a message to answer, so the row needs a contact to write to.
func classifyMeetingFollowUp(met AwaitedReply, asOf time.Time) ranked {
	days := daysSince(met.SentAt, asOf)
	row := crmcontracts.WorklistItem{
		Id:          met.ActivityID.String(),
		Source:      sourceMeetingFollowUp,
		Category:    crmcontracts.WorklistItemCategoryMeetings,
		Level:       levelPromise,
		Consequence: crmcontracts.WorklistItemConsequenceNone,
		Because: []crmcontracts.WorklistReason{
			reason("met_days_ago", daysValue(days)), reason("nothing_sent_since", nil),
		},
		Actions: []crmcontracts.WorklistItemActions{},
		Subject: waitingSubject(WaitingCustomer{
			DealID: met.DealID, ContactID: met.ContactID, CompanyID: met.CompanyID,
		}),
	}
	if met.Subject != "" {
		row.Title = &met.Subject
	}
	if !met.ContactID.IsZero() {
		row.Contact = &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(met.ContactID)}
	}
	if openableSubject(row.Subject) {
		row.Actions = append(row.Actions, crmcontracts.WorklistItemActions(actionOpen))
		if row.Contact != nil {
			row.Actions = append(row.Actions, crmcontracts.WorklistItemActionsReply)
		}
	}
	held := met.SentAt
	row.OccurredAt = &held
	return ranked{
		item: row, waitingDays: days, waitingRank: orderingAge(days), occurredAt: held,
		ownerRef: ownedByWhoeverIsReading(), contact: met.ContactID,
	}
}
