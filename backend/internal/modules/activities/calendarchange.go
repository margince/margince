// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A later calendar mail says what happened to a meeting: moved, called off, a
// new time proposed or an answer. EmailSummary.calendar_change carries it, so a
// client draws the meeting and what changed, not a mail to put down.
//
// The comparison is with the earlier send of the same event in the reader's
// own mailbox. Capture stores each send's event (activity.calendar_event).

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// calendarSend is one calendar mail's stored event, with the send before it.
type calendarSend struct {
	method string
	event  calendarevent.Event
	before *calendarevent.Event
}

// calendarSendsTx reads the stored events of the reader's own messages. Each
// comes with the latest earlier REQUEST or ADD of the same UID and occurrence.
// Failing that, it comes with the series invitation.
func calendarSendsTx(ctx context.Context, tx pgx.Tx, messages []ids.UUID, reader ids.UUID) (map[ids.UUID]calendarSend, error) {
	out := map[ids.UUID]calendarSend{}
	if len(messages) == 0 || reader.IsZero() {
		return out, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.calendar_method, a.calendar_event, prior.calendar_event
		  FROM activity a
		  LEFT JOIN LATERAL (
		       SELECT p.calendar_event FROM activity p
		        WHERE p.calendar_uid = a.calendar_uid AND p.kind = 'email' AND p.id <> a.id
		          AND p.calendar_event IS NOT NULL AND p.restricted_at IS NULL
		          AND p.calendar_method IN ('REQUEST', 'ADD')
		          AND coalesce(p.calendar_event->>'occurrence', '') IN ('', coalesce(a.calendar_event->>'occurrence', ''))
		          AND (p.occurred_at, p.id) < (a.occurred_at, a.id)
		          AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = p.id AND ci.user_id = $2)
		        ORDER BY coalesce(p.calendar_event->>'occurrence', '') = coalesce(a.calendar_event->>'occurrence', '') DESC,
		                 p.occurred_at DESC, p.id DESC
		        LIMIT 1) prior ON true
		 WHERE a.id = ANY($1) AND a.kind = 'email' AND a.direction = 'inbound'
		   AND a.restricted_at IS NULL AND a.calendar_uid IS NOT NULL
		   AND a.calendar_event IS NOT NULL AND a.calendar_method IS NOT NULL
		   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = $2)`,
		messages, reader)
	if err != nil {
		return nil, fmt.Errorf("activities: reading what calendar mail says of its meetings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var send calendarSend
		if err := rows.Scan(&id, &send.method, &send.event, &send.before); err != nil {
			return nil, fmt.Errorf("activities: reading what calendar mail says of its meetings: %w", err)
		}
		out[id] = send
	}
	return out, rows.Err()
}

// change is what the send tells the reader, or nil when it is a first
// invitation or a method that tells nothing.
func (s calendarSend) change() *crmcontracts.EmailCalendarChange {
	switch {
	case s.method == "CANCEL" || s.event.Canceled:
		return s.canceled()
	case s.method == "COUNTER":
		return s.proposed()
	case s.method == "REPLY":
		return s.answered()
	case s.method == "REQUEST" || s.method == "ADD":
		return s.updated()
	}
	return nil
}

// updated is a send after the first: a higher SEQUENCE, or an earlier send to
// read it against. A resend that changed nothing a reader sees, at the same
// revision, is still the invitation.
func (s calendarSend) updated() *crmcontracts.EmailCalendarChange {
	before := s.seriesAsOccurrence()
	if before == nil {
		before = s.occurrenceSlot()
	}
	if before == nil {
		if s.event.Sequence == 0 {
			return nil
		}
		return s.wire(crmcontracts.EmailCalendarChangeKindUpdated, calendarevent.Difference{})
	}
	diff := calendarevent.Compare(*before, s.event)
	if !diff.Any() && s.event.Sequence <= before.Sequence {
		return nil
	}
	out := s.wire(crmcontracts.EmailCalendarChangeKindUpdated, diff)
	out.Before = beforeWire(*before, diff)
	return out
}

// occurrenceSlot is where one meeting of a series stood before this send moved
// it: RECURRENCE-ID is that meeting's original start. Nil for a series or a
// one-off, and for a meeting this send did not move.
func (s calendarSend) occurrenceSlot() *calendarevent.Event {
	slot, allDay, ok := occurrenceStart(s.event.Occurrence)
	if !ok || s.event.Start == nil || allDay != s.event.AllDay || slot.Equal(*s.event.Start) {
		return nil
	}
	return &calendarevent.Event{
		Sequence: s.event.Sequence, Title: s.event.Title, Location: s.event.Location,
		Start: &slot, AllDay: allDay, Attendees: s.event.Attendees, AttendeesCut: s.event.AttendeesCut,
	}
}

// seriesAsOccurrence is the send before, read as this one meeting of it. A
// series invitation starts when the series does, so the meeting's start stands in.
func (s calendarSend) seriesAsOccurrence() *calendarevent.Event {
	if s.before == nil || s.before.Occurrence != "" || s.event.Occurrence == "" {
		return s.before
	}
	before := *s.before
	before.Start, before.End = nil, nil
	if slot, allDay, ok := occurrenceStart(s.event.Occurrence); ok {
		before.Start, before.AllDay = &slot, allDay
		if s.before.Start != nil && s.before.End != nil {
			end := slot.Add(s.before.End.Sub(*s.before.Start))
			before.End = &end
		}
	}
	return &before
}

// occurrenceStart reads an occurrence key: an instant, or an all-day date.
func occurrenceStart(key string) (time.Time, bool, bool) {
	if at, err := time.Parse(time.RFC3339, key); err == nil {
		return at, false, true
	}
	if day, err := time.Parse("20060102", key); err == nil {
		return day, true, true
	}
	return time.Time{}, false, false
}

// canceled is the meeting called off, with what the invitation said of it
// where the cancellation leaves a detail out.
func (s calendarSend) canceled() *crmcontracts.EmailCalendarChange {
	shown := s
	if before := s.seriesAsOccurrence(); before != nil {
		if shown.event.Title == "" {
			shown.event.Title = before.Title
		}
		if shown.event.Location == "" {
			shown.event.Location = before.Location
		}
		if shown.event.Start == nil {
			shown.event.Start, shown.event.End, shown.event.AllDay = before.Start, before.End, before.AllDay
		}
	}
	return shown.wire(crmcontracts.EmailCalendarChangeKindCanceled, calendarevent.Difference{})
}

// proposed is a guest's counter-proposal: the time it names, against the
// meeting's time in the send before it.
func (s calendarSend) proposed() *crmcontracts.EmailCalendarChange {
	var diff calendarevent.Difference
	before := s.seriesAsOccurrence()
	if before != nil {
		diff.Time = calendarevent.Compare(*before, s.event).Time
	}
	out := s.wire(crmcontracts.EmailCalendarChangeKindProposed, diff)
	if before != nil {
		out.Before = beforeWire(*before, diff)
	}
	out.By = firstGuest(s.event)
	return out
}

// answered is a guest's RSVP. Their part names them with their answer.
func (s calendarSend) answered() *crmcontracts.EmailCalendarChange {
	for _, guest := range s.event.Attendees {
		if guest.Answer == "" {
			continue
		}
		out := s.wire(crmcontracts.EmailCalendarChangeKindAnswered, calendarevent.Difference{})
		out.By = guestWire(guest)
		answer := crmcontracts.EmailCalendarChangeAnswer(guest.Answer)
		out.Answer = &answer
		return out
	}
	return nil
}

func firstGuest(event calendarevent.Event) *crmcontracts.EmailCalendarGuest {
	if len(event.Attendees) == 0 {
		return nil
	}
	return guestWire(event.Attendees[0])
}

// wire is the meeting as this send states it, with what diff says changed.
func (s calendarSend) wire(kind crmcontracts.EmailCalendarChangeKind, diff calendarevent.Difference) *crmcontracts.EmailCalendarChange {
	out := &crmcontracts.EmailCalendarChange{Kind: kind, Changed: []crmcontracts.EmailCalendarChangeChanged{}}
	out.Title, out.Location = nonEmpty(s.event.Title), nonEmpty(s.event.Location)
	out.StartsAt, out.EndsAt, out.StartsPrecision = eventTimes(s.event)
	if diff.Time {
		out.Changed = append(out.Changed, crmcontracts.EmailCalendarChangeChangedTime)
	}
	if diff.Location {
		out.Changed = append(out.Changed, crmcontracts.EmailCalendarChangeChangedLocation)
	}
	if diff.Title {
		out.Changed = append(out.Changed, crmcontracts.EmailCalendarChangeChangedTitle)
	}
	out.GuestsAdded, out.GuestsRemoved = guestsWire(diff.Added), guestsWire(diff.Removed)
	return out
}

// beforeWire carries the earlier value of each field diff names.
func beforeWire(before calendarevent.Event, diff calendarevent.Difference) *crmcontracts.EmailCalendarBefore {
	if !diff.Time && !diff.Location && !diff.Title {
		return nil
	}
	out := &crmcontracts.EmailCalendarBefore{}
	if diff.Time {
		out.StartsAt, out.EndsAt, out.StartsPrecision = eventTimes(before)
	}
	if diff.Location {
		out.Location = nonEmpty(before.Location)
	}
	if diff.Title {
		out.Title = nonEmpty(before.Title)
	}
	return out
}

func eventTimes(event calendarevent.Event) (*time.Time, *time.Time, *crmcontracts.OccurredPrecision) {
	if event.Start == nil {
		return nil, nil, nil
	}
	precision := crmcontracts.OccurredPrecisionOccurredPrecisionInstant
	if event.AllDay {
		precision = crmcontracts.OccurredPrecisionOccurredPrecisionDay
	}
	return event.Start, event.End, &precision
}

func guestsWire(guests []calendarevent.Attendee) *[]crmcontracts.EmailCalendarGuest {
	if len(guests) == 0 {
		return nil
	}
	out := make([]crmcontracts.EmailCalendarGuest, 0, len(guests))
	for _, guest := range guests {
		out = append(out, *guestWire(guest))
	}
	return &out
}

func guestWire(guest calendarevent.Attendee) *crmcontracts.EmailCalendarGuest {
	return &crmcontracts.EmailCalendarGuest{Email: guest.Email, Name: nonEmpty(guest.Name)}
}

func nonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// withCalendarChanges marks the page's calendar mail that tells the reader
// something about a meeting. Only a row the projection calls a calendar mail
// can, so a page without one reads nothing more.
func withCalendarChanges(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	calendarMail := calendarMailOf(page)
	if len(calendarMail) == 0 {
		return nil
	}
	sends, err := calendarSendsTx(ctx, tx, calendarMail, readerOrNobody(ctx))
	if err != nil || len(sends) == 0 {
		return err
	}
	for i := range page {
		if send, ok := sends[ids.UUID(page[i].Id)]; ok && page[i].EmailSummary != nil {
			page[i].EmailSummary.CalendarChange = send.change()
		}
	}
	return nil
}

// markCalendarChange is withCalendarChanges for the one message a reader opened.
func markCalendarChange(ctx context.Context, tx pgx.Tx, id ids.ActivityID, summary *crmcontracts.EmailSummary) error {
	sends, err := calendarSendsTx(ctx, tx, []ids.UUID{id.UUID}, readerOrNobody(ctx))
	if err != nil {
		return err
	}
	if send, ok := sends[id.UUID]; ok {
		summary.CalendarChange = send.change()
	}
	return nil
}
