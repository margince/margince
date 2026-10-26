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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
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
	// Recipients is who a meeting follow-up goes to, ContactID first.
	Recipients []ids.UUID
	// Precision says SentAt is a date's anchor when it is day.
	Precision crmcontracts.OccurredPrecision
	// Present when the reader may read the message, which is what the
	// composer drafts the follow-up from.
	EmailSummary *crmcontracts.EmailSummary
}

// sent is SentAt read by its precision, which is what a day count needs.
func (a AwaitedReply) sent() occurredday.Moment {
	return occurredday.Moment{At: a.SentAt, DateOnly: occurredday.IsDay(a.Precision)}
}

// followUpRead is the reader's follow-ups, read per request like planRows:
// read says the source ran, cut that it stopped at its bound.
type followUpRead struct {
	rows         []ranked
	read         bool
	cut          bool
	meetingsRead bool
	meetingsCut  bool
}

// bound records each follow-up source that ran, and whether it hit its cap.
func (f followUpRead) bound(bounded map[crmcontracts.WorklistItemSource]bool) {
	if f.read {
		bounded[sourceAwaitingReply] = f.cut
	}
	if f.meetingsRead {
		bounded[sourceMeetingFollowUp] = f.meetingsCut
	}
}

// WithAwaiting binds the follow-up reader. Unbound, the source is absent.
func (s *Service) WithAwaiting(a Awaiting) *Service {
	s.awaiting = a
	return s
}

// readingAwaiting reads the reader's follow-ups onto a per-read copy of the
// service, the way readingPlan carries the plan rows. The rows are the
// reader's own. They ride the reader's own day and the team and all views
// that include it, never a colleague's queue or the unassigned one.
// The two reads fail apart, so one stumbling keeps the other's rows.
func (s *Service) readingAwaiting(ctx context.Context, asOf time.Time, loc *time.Location) (*Service, []*crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	if s.awaiting == nil || s.taskScope == TasksOwnedBy || s.taskScope == TasksUnassigned {
		return &scoped, nil
	}
	replies, cut, repliesErr := s.readFollowUps(ctx, sourceAwaitingReply, asOf, s.awaiting.AwaitingReplies)
	meetings, meetingsCut, meetingsErr := s.readFollowUps(ctx, sourceMeetingFollowUp, asOf, s.awaiting.MeetingFollowUps)
	scoped.followUps = followUpRead{
		rows: make([]ranked, 0, len(replies)+len(meetings)),
		read: repliesErr == nil, cut: cut, meetingsRead: meetingsErr == nil, meetingsCut: meetingsCut,
	}
	for _, row := range replies {
		scoped.followUps.rows = append(scoped.followUps.rows, classifyAwaiting(row, asOf, loc))
	}
	for _, meeting := range meetings {
		if row, actionable := classifyMeetingFollowUp(meeting, asOf, loc); actionable {
			scoped.followUps.rows = append(scoped.followUps.rows, row)
		}
	}
	return &scoped, []*crmcontracts.WorklistSourceUnavailable{repliesErr, meetingsErr}
}

func (s *Service) readFollowUps(
	ctx context.Context, source string, asOf time.Time,
	read func(context.Context, time.Time) ([]AwaitedReply, bool, error),
) ([]AwaitedReply, bool, *crmcontracts.WorklistSourceUnavailable) {
	var rows []AwaitedReply
	var cut bool
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		rows, cut, err = read(ctx, asOf)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return nil, false, &crmcontracts.WorklistSourceUnavailable{
			Source: source, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
		}
	case err != nil:
		slog.Log(ctx, laneFailureLevel(ctx), "a follow-up read failed", "source", source, "error", err)
		return nil, false, &crmcontracts.WorklistSourceUnavailable{
			Source: source, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	}
	return rows, cut, nil
}

// classifyAwaiting is one follow-up row. It is the reader's own promise to
// keep the conversation going, so it ranks with promises, below a customer
// who is waiting on them.
func classifyAwaiting(awaited AwaitedReply, asOf time.Time, loc *time.Location) ranked {
	days := calendarDaysSince(awaited.sent(), asOf, loc)
	row := crmcontracts.WorklistItem{
		Id:           awaited.ActivityID.String(),
		Source:       sourceAwaitingReply,
		SourceRecord: subjectOf("activity", awaited.ActivityID),
		Category:     crmcontracts.WorklistItemCategoryTasks,
		Level:        levelPromise,
		Consequence:  crmcontracts.WorklistItemConsequenceNone,
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
	row.OccurredPrecision = precisionOf(&awaited.Precision)
	anchor := openapi_types.UUID(awaited.ActivityID)
	row.Move = &crmcontracts.WorklistMove{Action: crmcontracts.WorklistMoveActionDraftReply, ActivityId: &anchor}
	return ranked{
		item: row, waitingDays: days, waitingRank: orderingAge(days), occurredAt: sent,
		ownerRef: ownedByWhoeverIsReading(), contact: awaited.ContactID,
	}
}

// classifyMeetingFollowUp is one meeting the reader owes a follow-up. A
// meeting is not a message to answer, so the composer writes a fresh message
// to everyone it was with. With nobody on file the row opens the deal or the
// company instead; with neither there is nothing to act on, and no row.
func classifyMeetingFollowUp(met AwaitedReply, asOf time.Time, loc *time.Location) (ranked, bool) {
	days := calendarDaysSince(met.sent(), asOf, loc)
	row := crmcontracts.WorklistItem{
		Id:           met.ActivityID.String(),
		Source:       sourceMeetingFollowUp,
		SourceRecord: subjectOf("activity", met.ActivityID),
		Category:     crmcontracts.WorklistItemCategoryMeetings,
		Level:        levelPromise,
		Consequence:  crmcontracts.WorklistItemConsequenceNone,
		Because: []crmcontracts.WorklistReason{
			reason("met_days_ago", daysValue(days)), reason("nothing_sent_since", nil),
		},
		Actions: []crmcontracts.WorklistItemActions{},
		Subject: waitingSubject(WaitingCustomer{
			DealID: met.DealID, ContactID: met.ContactID, CompanyID: met.CompanyID,
		}),
	}
	if !openableSubject(row.Subject) {
		return ranked{}, false
	}
	if met.Subject != "" {
		row.Title = &met.Subject
	}
	row.Actions = append(row.Actions, crmcontracts.WorklistItemActions(actionOpen))
	if len(met.Recipients) > 0 {
		recipients := make([]crmcontracts.WorklistContactFacts, 0, len(met.Recipients))
		for _, id := range met.Recipients {
			recipients = append(recipients, crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(id)})
		}
		row.Recipients = &recipients
		row.Contact = &crmcontracts.WorklistContactFacts{Id: openapi_types.UUID(met.Recipients[0])}
		row.Actions = append(row.Actions, crmcontracts.WorklistItemActionsReply)
	} else {
		row.Because = append(row.Because, reason("no_attendee_on_file", nil))
	}
	held := met.SentAt
	row.OccurredAt = &held
	row.OccurredPrecision = precisionOf(&met.Precision)
	return ranked{
		item: row, waitingDays: days, waitingRank: orderingAge(days), occurredAt: held,
		ownerRef: ownedByWhoeverIsReading(), contact: met.ContactID,
	}, true
}
