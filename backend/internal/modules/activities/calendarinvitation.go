// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A calendar invitation that came by email asks for an RSVP, never for a
// written reply. The row says so (EmailSummary.calendar_invitation) and the
// reader answers it here, on their own calendar, which tells the organizer.
//
// The invitation and the meeting the calendar connector captured meet on the
// event's iCalendar UID: the mail parser stores it as activity.calendar_uid,
// and a meeting's identity key starts with it (MeetingIdentityKey).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/calendarmethod"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// openInvitation is one invitation the reader may still answer.
type openInvitation struct {
	uid      string
	meeting  *ids.UUID
	response crmcontracts.EmailCalendarInvitationResponse
	startsAt *time.Time
}

func (o openInvitation) wire() crmcontracts.EmailCalendarInvitation {
	return crmcontracts.EmailCalendarInvitation{Response: o.response, StartsAt: o.startsAt}
}

// openInvitationsTx reads which of these messages are invitations this reader
// may still answer. Only the reader's own mailbox received an invitation to
// them, so a colleague reading the same row is offered no answer. An event
// that has started, or was cancelled, is no longer asking.
//
// The meeting is the event's next occurrence, or its latest when none is
// ahead, so a weekly series reads the answer the reader gave it last.
func openInvitationsTx(
	ctx context.Context, tx pgx.Tx, messages []ids.UUID, reader ids.UUID, asOf time.Time,
) (map[ids.UUID]openInvitation, error) {
	out := map[ids.UUID]openInvitation{}
	if len(messages) == 0 || reader.IsZero() {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.calendar_uid, m.id, m.occurred_at,
		       (SELECT p.response FROM activity_participant p
		         WHERE p.activity_id = m.id AND p.user_id = $2 AND p.response IS NOT NULL
		         ORDER BY p.created_at LIMIT 1)
		  FROM activity a
		  LEFT JOIN LATERAL (
		       SELECT mt.id, mt.occurred_at, mt.meeting_status
		         FROM activity_identity ai JOIN activity mt ON mt.id = ai.activity_id
		        WHERE ai.identity_kind = '`+IdentityKindMeeting+`'
		          AND `+meetingUIDOfKeySQL("ai.identity_key")+` = a.calendar_uid
		          AND mt.kind = 'meeting' AND mt.archived_at IS NULL AND mt.restricted_at IS NULL
		        ORDER BY mt.occurred_at < $3,
		                 CASE WHEN mt.occurred_at >= $3 THEN mt.occurred_at END,
		                 mt.occurred_at DESC
		        LIMIT 1) m ON true
		 WHERE a.id = ANY($1) AND a.kind = 'email' AND a.direction = 'inbound'
		   AND a.calendar_uid IS NOT NULL AND a.restricted_at IS NULL
		   AND a.calendar_method IN ('`+strings.Join(calendarmethod.Answerable(), "', '")+`')
		   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = $2)
		   AND (m.id IS NULL OR (m.occurred_at >= $3 AND m.meeting_status IS DISTINCT FROM 'canceled'))`,
		messages, reader, asOf)
	if err != nil {
		return nil, fmt.Errorf("activities: reading which messages are open invitations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var found openInvitation
		var response *string
		if err := rows.Scan(&id, &found.uid, &found.meeting, &found.startsAt, &response); err != nil {
			return nil, fmt.Errorf("activities: reading which messages are open invitations: %w", err)
		}
		found.response = crmcontracts.EmailCalendarInvitationResponseNeedsAction
		if response != nil {
			found.response = crmcontracts.EmailCalendarInvitationResponse(*response)
		}
		out[id] = found
	}
	return out, rows.Err()
}

// withCalendarInvitations marks the page's readable emails that are open
// invitations to the reader.
func withCalendarInvitations(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity, emailIDs []ids.UUID, asOf time.Time) error {
	open, err := openInvitationsTx(ctx, tx, emailIDs, readerOrNobody(ctx), asOf)
	if err != nil || len(open) == 0 {
		return err
	}
	for i := range page {
		found, ok := open[ids.UUID(page[i].Id)]
		if ok && page[i].EmailSummary != nil {
			invitation := found.wire()
			page[i].EmailSummary.CalendarInvitation = &invitation
		}
	}
	return nil
}

// errNotAnOpenInvitation answers a message the reader cannot RSVP to: not an
// invitation, not one their mailbox received, or one whose event has begun.
func errNotAnOpenInvitation() error {
	return &values.ParseError{
		Field: fieldRowID, Code: "not_an_open_invitation",
		Message: "this message is not an invitation you can still answer; answer it in your calendar",
	}
}

// eventMissingAsRefusal names a calendar that does not hold the invited event,
// which the reader can act on: the invitation went to another calendar.
func eventMissingAsRefusal(err error) error {
	if !errors.Is(err, connector.ErrCalendarEventMissing) {
		return err
	}
	return &values.ParseError{
		Field: fieldRowID, Code: "event_not_on_calendar",
		Message: "your connected calendar does not hold this event; answer it in the calendar it was sent to",
	}
}

// AnswerCalendarInvitation sends the reader's answer to the event an
// invitation names, through their own calendar, and records it.
//
// The provider call sits between two transactions rather than inside one, so
// no row lock is held across the network. The second re-reads the invitation,
// and a message that stopped being one in between records nothing.
func (s *Store) AnswerCalendarInvitation(
	ctx context.Context, id ids.ActivityID, response crmcontracts.AnswerCalendarInvitationRequestResponse,
) (crmcontracts.EmailCalendarInvitation, error) {
	reader := readerOrNobody(ctx)
	if reader.IsZero() || s.calendar == nil {
		return crmcontracts.EmailCalendarInvitation{}, apperrors.ErrPermissionDenied
	}
	if !response.Valid() {
		return crmcontracts.EmailCalendarInvitation{}, &values.ParseError{
			Field: "response", Code: "unknown_response", Message: "answer accepted, tentative or declined",
		}
	}
	var invitation openInvitation
	if err := s.judgeMessage(ctx, id, func(ctx context.Context, tx pgx.Tx, _ string) error {
		var err error
		invitation, err = s.openInvitationTx(ctx, tx, id, reader)
		return err
	}); err != nil {
		return crmcontracts.EmailCalendarInvitation{}, err
	}
	if err := s.calendar.Respond(ctx, ids.From[ids.UserKind](reader), invitation.uid, string(response)); err != nil {
		return crmcontracts.EmailCalendarInvitation{}, eventMissingAsRefusal(unconnectedAsRefusal(err))
	}
	err := s.judgeMessage(ctx, id, func(ctx context.Context, tx pgx.Tx, capturedBy string) error {
		current, err := s.openInvitationTx(ctx, tx, id, reader)
		if err != nil {
			return err
		}
		if err := recordInvitationAnswerTx(ctx, tx, id, current, reader, response); err != nil {
			return err
		}
		return s.writeReaderState(id, stateHandled, nil, "", nil, ensureInboundMessage)(ctx, tx, capturedBy)
	})
	invitation.response = crmcontracts.EmailCalendarInvitationResponse(response)
	return invitation.wire(), err
}

func (s *Store) openInvitationTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, reader ids.UUID) (openInvitation, error) {
	open, err := openInvitationsTx(ctx, tx, []ids.UUID{id.UUID}, reader, s.now())
	if err != nil {
		return openInvitation{}, err
	}
	found, ok := open[id.UUID]
	if !ok {
		return openInvitation{}, errNotAnOpenInvitation()
	}
	return found, nil
}

// recordInvitationAnswerTx stores the answer on the reader's own place in the
// meeting, so the row reads it before the calendar next syncs, and writes the
// audit row and the announcement beside it.
func recordInvitationAnswerTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, invitation openInvitation, reader ids.UUID,
	response crmcontracts.AnswerCalendarInvitationRequestResponse,
) error {
	if invitation.meeting != nil {
		if _, err := tx.Exec(ctx, `UPDATE activity_participant SET response = $3
			 WHERE activity_id = $1 AND user_id = $2`, *invitation.meeting, reader, string(response)); err != nil {
			return fmt.Errorf("activities: recording the reader's answer on the meeting: %w", err)
		}
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", "activity", id.UUID,
		map[string]any{"invitation_response": string(response)})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityInvitationAnswered{
		ActivityId: openapi_types.UUID(id.UUID),
		Response:   crmcontracts.PublicEventActivityInvitationAnsweredResponse(response),
	})
}
