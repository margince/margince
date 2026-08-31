// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// This file is the Google Calendar half of the calendar mapping: it decodes a
// Calendar v3 event resource into the provider-neutral capture/meetingmap.Event
// and does nothing else. The RULES — which events are worth logging, who counts
// as a party, what the stored activity looks like — live in meetingmap, because
// they are answers about MEETINGS rather than about Google, and a second copy
// would be a second answer.
//
// Pure: no provider handle, no I/O beyond reading the in-memory event bytes.

package gcal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rawEvent is the subset of a Google Calendar v3 event resource this decode
// reads. Unknown fields are ignored — the raw original is stored verbatim as
// evidence (memory-first), so nothing is lost by reading only what we use.
type rawEvent struct {
	ID          string        `json:"id"`
	Status      string        `json:"status"` // "confirmed" | "tentative" | "cancelled"
	Summary     string        `json:"summary"`
	Description string        `json:"description"`
	Start       eventDateTime `json:"start"`
	Organizer   eventActor    `json:"organizer"`
	Attendees   []eventActor  `json:"attendees"`
}

// eventDateTime is a calendar timestamp: dateTime (RFC3339) for a timed event,
// or date (YYYY-MM-DD) for an all-day one.
type eventDateTime struct {
	DateTime string `json:"dateTime"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Date     string `json:"date"`
}

// eventActor is one organizer/attendee: the email is all the mapping needs to
// resolve the counterparty, plus Google's resource flag, which marks a booked
// room or device rather than a person.
type eventActor struct {
	Email    string `json:"email"`
	Resource bool   `json:"resource"`
}

// roomResourceDomain is where Google Calendar homes every booked room and
// device. An event stored before the resource flag was read carries the address
// alone, so the domain is recognised as well as the flag.
const roomResourceDomain = "resource.calendar.google.com"

// isRoom reports whether this attendee is a booked room or device rather than a
// person. A room is not a party to a meeting: it cannot be a counterparty, it
// cannot be answered, and — the case that matters — it is on no workspace's own
// domain, so counting it would make every all-colleague meeting held in a
// booked room look like it had an outside guest.
func (a eventActor) isRoom() bool {
	if a.Resource {
		return true
	}
	dom := domainOf(a.Email)
	return dom == roomResourceDomain || strings.HasSuffix(dom, "."+roomResourceDomain)
}

// decodeEvent reads one raw Calendar event resource into the neutral shape —
// this connector's whole contribution to the calendar mapping.
func decodeEvent(raw []byte) (meetingmap.Event, error) {
	var ev rawEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return meetingmap.Event{}, fmt.Errorf("gcal: parsing calendar event: %w", err)
	}
	return decode(ev), nil
}

// decode maps Google's event resource onto the neutral shape.
func decode(ev rawEvent) meetingmap.Event {
	attendees := make([]meetingmap.Actor, 0, len(ev.Attendees))
	for _, a := range ev.Attendees {
		attendees = append(attendees, meetingmap.Actor{Email: a.Email, Room: a.isRoom()})
	}
	return meetingmap.Event{
		ID:          ev.ID,
		Cancelled:   strings.EqualFold(strings.TrimSpace(ev.Status), "cancelled"),
		Subject:     ev.Summary,
		Description: ev.Description,
		StartsAt:    parseStart(ev.Start),
		Organizer:   meetingmap.Actor{Email: ev.Organizer.Email},
		Attendees:   attendees,
	}
}

// ParticipantsOf reads the organizer and attendees out of one stored event
// resource, for the replay pass that recovers meetings captured before
// participants were recorded.
func ParticipantsOf(raw []byte, owner string) ([]connector.MessageParticipant, error) {
	return meetingmap.ParticipantsOf(raw, owner, decodeEvent)
}

// parseStart reads the event's start: a timed dateTime (RFC3339) preferred,
// falling back to an all-day date. A start we cannot read yields the zero time
// — the Sink then stamps capture time honestly rather than sorting the row to
// the beginning of history.
func parseStart(start eventDateTime) time.Time {
	if dt := strings.TrimSpace(start.DateTime); dt != "" {
		if t, err := time.Parse(time.RFC3339, dt); err == nil {
			return t.UTC()
		}
	}
	if t, ok := meetingmap.AllDayStart(start.Date); ok {
		return t
	}
	return time.Time{}
}

// domainOf returns the lowercased domain part of an address, or "" if it
// carries no "@". It splits at the LAST "@" so a quoted local part containing
// one still yields the domain.
func domainOf(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if idx := strings.LastIndex(addr, "@"); idx >= 0 {
		return addr[idx+1:]
	}
	return ""
}
