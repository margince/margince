// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This file is the Microsoft half of the calendar mapping: it decodes a Graph
// event resource into the provider-neutral capture/meetingmap.Event and does
// nothing else. The RULES — which events are worth logging, who counts as a
// party, what the stored activity looks like — live in meetingmap, shared with
// the Google Calendar connector, because they are answers about MEETINGS rather
// than about Microsoft.
//
// Pure: no provider handle, no I/O beyond reading the in-memory event bytes.

package graphcal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rawEvent is the subset of a Graph event resource this decode reads. Unknown
// fields are ignored — the raw original is stored verbatim as evidence
// (memory-first), so nothing is lost by reading only what we use.
type rawEvent struct {
	ID          string       `json:"id"`
	Subject     string       `json:"subject"`
	BodyPreview string       `json:"bodyPreview"` //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
	IsCancelled bool         `json:"isCancelled"` //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
	IsAllDay    bool         `json:"isAllDay"`    //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
	Start       graphTime    `json:"start"`
	Organizer   graphActor   `json:"organizer"`
	Attendees   []graphActor `json:"attendees"`
	// Removed is Graph's tombstone on a delta round: an event deleted since the
	// last pull arrives carrying this and little else. It is read as a
	// cancellation because that is what it is to a calendar, and the shared
	// rules already drop a cancelled meeting.
	Removed *struct{} `json:"@removed"`
}

// graphTime is a Graph calendar timestamp: a local wall time plus the zone it
// is stated in, never an offset — so it needs the pair to become an instant.
type graphTime struct {
	DateTime string `json:"dateTime"` //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
	TimeZone string `json:"timeZone"` //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
}

// graphActor is one organizer/attendee. Microsoft nests the address one level
// down and states an attendee's KIND separately — "resource" is a booked room
// or device rather than a person.
type graphActor struct {
	Type         string `json:"type"`
	EmailAddress struct {
		Address string `json:"address"`
	} `json:"emailAddress"` //nolint:tagliatelle // Microsoft's wire format (camelCase); must match to decode
}

// parseEvent reads one raw Graph event resource and classifies it against the
// calendar owner (whose domain marks "internal").
func parseEvent(raw []byte, owner string) (meetingmap.Meeting, error) {
	var ev rawEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return meetingmap.Meeting{}, fmt.Errorf("graphcal: parsing calendar event: %w", err)
	}
	return meetingmap.Classify(decode(ev), owner), nil
}

// decode maps Microsoft's event resource onto the neutral shape.
func decode(ev rawEvent) meetingmap.Event {
	attendees := make([]meetingmap.Actor, 0, len(ev.Attendees))
	for _, a := range ev.Attendees {
		attendees = append(attendees, meetingmap.Actor{
			Email: a.EmailAddress.Address,
			Room:  strings.EqualFold(strings.TrimSpace(a.Type), "resource"),
		})
	}
	return meetingmap.Event{
		ID:          ev.ID,
		Cancelled:   ev.IsCancelled || ev.Removed != nil,
		Subject:     ev.Subject,
		Description: ev.BodyPreview,
		StartsAt:    parseStart(ev.Start, ev.IsAllDay),
		Organizer:   meetingmap.Actor{Email: ev.Organizer.EmailAddress.Address},
		Attendees:   attendees,
	}
}

// ParticipantsOf reads the organizer and attendees out of one stored event
// resource — the calendar twin of mailmap.ParticipantsOf, for the replay pass
// that recovers meetings captured before participants were recorded.
func ParticipantsOf(raw []byte, owner string) ([]connector.MessageParticipant, error) {
	m, err := parseEvent(raw, owner)
	if err != nil {
		return nil, err
	}
	return m.Participants(), nil
}

// graphLocalLayouts are the wall-clock forms Graph states a calendar time in.
// It sends fractional seconds and no offset, which is why time.RFC3339 does not
// read it: the zone travels beside the value, in timeZone.
var graphLocalLayouts = []string{
	"2006-01-02T15:04:05.9999999",
	"2006-01-02T15:04:05",
}

// parseStart resolves the event's start to an instant.
//
// An all-day event is anchored at noon in the shared way rather than at the
// midnight Graph states, for the reason meetingmap.AllDayStart gives — the date
// is what the event means, and midnight in the wrong zone is the previous day.
//
// A timed event is a wall clock plus a named zone. The zone is loaded from the
// system database; a name it cannot resolve (Microsoft still emits Windows zone
// ids on some calendars, "W. Europe Standard Time" rather than "Europe/Berlin")
// yields the zero time rather than a wrong instant, and the Sink then stamps
// capture time honestly instead of filing the meeting hours from where it was.
func parseStart(start graphTime, allDay bool) time.Time {
	value := strings.TrimSpace(start.DateTime)
	if value == "" {
		return time.Time{}
	}
	if allDay {
		date, _, _ := strings.Cut(value, "T")
		if t, ok := meetingmap.AllDayStart(date); ok {
			return t
		}
		return time.Time{}
	}
	loc, err := time.LoadLocation(strings.TrimSpace(start.TimeZone))
	if err != nil {
		return time.Time{}
	}
	for _, layout := range graphLocalLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
