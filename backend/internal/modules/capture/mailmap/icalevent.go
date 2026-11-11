// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strconv"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// Bounds on what a sender can make one row store; real events stay far under.
const (
	maxEventTitle    = 500
	maxEventLocation = 500
	maxAttendeeName  = 200
)

// eventOf is the first VEVENT inside a calendar body, or nil when it holds
// none. Only the first is read, for the reason eventUID gives. A component
// nested in it, such as a VALARM, has properties of its own and is skipped.
func eventOf(body []byte) *calendarevent.Event {
	lines := icalLines(body)
	zones := readZones(lines)
	var event *calendarevent.Event
	var start, end *icalLine
	depth := 0
	for i, line := range lines {
		switch {
		case event == nil:
			if line.is("BEGIN") && strings.EqualFold(strings.TrimSpace(line.value), "VEVENT") {
				event = &calendarevent.Event{}
			}
		case line.is("BEGIN"):
			depth++
		case line.is("END") && depth > 0:
			depth--
		case line.is("END"):
			return withTimes(event, start, end, zones)
		case depth > 0:
		case line.is("DTSTART"):
			start = &lines[i]
		case line.is("DTEND"):
			end = &lines[i]
		default:
			readEventProperty(event, line)
		}
	}
	if event == nil {
		return nil
	}
	return withTimes(event, start, end, zones)
}

func withTimes(event *calendarevent.Event, start, end *icalLine, zones statedZones) *calendarevent.Event {
	if start == nil {
		return event
	}
	at, allDay, ok := icalTime(*start, zones)
	if !ok {
		return event
	}
	event.Start, event.AllDay = &at, allDay
	if end != nil {
		if until, _, ok := icalTime(*end, zones); ok && until.After(at) {
			event.End = &until
		}
	}
	return event
}

// readEventProperty takes one property of the VEVENT itself.
func readEventProperty(event *calendarevent.Event, line icalLine) {
	value := strings.TrimSpace(line.value)
	switch strings.ToUpper(line.name) {
	case "SEQUENCE":
		if sequence, err := strconv.Atoi(value); err == nil && sequence >= 0 {
			event.Sequence = sequence
		}
	case "SUMMARY":
		event.Title = textcut.Runes(icalText(value), maxEventTitle)
	case "LOCATION":
		event.Location = textcut.Runes(icalText(value), maxEventLocation)
	case "STATUS":
		event.Canceled = strings.EqualFold(value, "CANCELLED")
	case "RECURRENCE-ID":
		event.Occurrence = recurrenceKey(line)
	case "ORGANIZER":
		event.Organizer = icalAddress(value)
	case "ATTENDEE":
		addAttendee(event, line)
	}
}

func addAttendee(event *calendarevent.Event, line icalLine) {
	email := icalAddress(line.value)
	if email == "" || !strings.Contains(email, "@") {
		return
	}
	if len(event.Attendees) >= calendarevent.MaxAttendees {
		event.AttendeesCut = true
		return
	}
	event.Attendees = append(event.Attendees, calendarevent.Attendee{
		Email:  email,
		Name:   textcut.Runes(strings.TrimSpace(line.param("CN")), maxAttendeeName),
		Answer: calendarevent.AnswerOf(line.param("PARTSTAT")),
	})
}

// recurrenceKey names one occurrence of a series the same way however the
// calendar spelled it: the instant in UTC, or the date of an all-day series.
func recurrenceKey(line icalLine) string {
	at, allDay, ok := icalTime(line, nil)
	switch {
	case !ok:
		return strings.TrimSpace(line.value)
	case allDay:
		return at.Format(icalDate)
	}
	return at.UTC().Format(time.RFC3339)
}
