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

// eventActor is one organizer/attendee: the email the mapping resolves the
// counterparty by, Google's resource flag marking a booked room or device
// rather than a person, and the name the organizer typed for them.
//
// The display name is the only place an attendee is named in full. A person
// minted from a bare invitation address is otherwise named by its local part,
// and no later pass has better evidence to correct it with.
type eventActor struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Resource    bool   `json:"resource"`
	// Self marks the attendee entry belonging to the calendar this event was
	// read from. It is Google's own answer to "which of these is you", which
	// beats matching the owner's address: the account may be invited under an
	// alias, or through a group, and would then match nothing.
	Self bool `json:"self"`
	// ResponseStatus is this attendee's RSVP — "needsAction", "declined",
	// "tentative" or "accepted".
	ResponseStatus string `json:"responseStatus"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
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
//
// The owner is needed to read the RSVP: "did I decline this" is a question
// about one attendee among many, and which one is the owner is not something
// the event says on its own in every case.
func decodeEvent(raw []byte, owner string) (meetingmap.Event, error) {
	var ev rawEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return meetingmap.Event{}, fmt.Errorf("gcal: parsing calendar event: %w", err)
	}
	return decode(ev, owner), nil
}

// decode maps Google's event resource onto the neutral shape.
func decode(ev rawEvent, owner string) meetingmap.Event {
	attendees := make([]meetingmap.Actor, 0, len(ev.Attendees))
	for _, a := range ev.Attendees {
		attendees = append(attendees, meetingmap.Actor{Email: a.Email, Name: a.DisplayName, Room: a.isRoom()})
	}
	return meetingmap.Event{
		ID:            ev.ID,
		Cancelled:     strings.EqualFold(strings.TrimSpace(ev.Status), "cancelled"),
		OwnerDeclined: ownerDeclined(ev.Attendees, owner),
		Subject:       ev.Summary,
		Description:   ev.Description,
		StartsAt:      parseStart(ev.Start),
		Organizer:     meetingmap.Actor{Email: ev.Organizer.Email, Name: ev.Organizer.DisplayName},
		Attendees:     attendees,
	}
}

// ownerDeclined reports that the connected account answered NO to this
// invitation.
//
// Google's `self` flag is the authority and the address is the FALLBACK, in
// that order and never as a pair of equals. The flag is the account's own answer
// to which attendee it is, so it holds when the invitation went to an alias or
// reached the account through a group — cases where matching the owner's address
// finds nobody at all. The address answers only for a payload carrying no flag.
//
// Asking them as one condition would let either say yes, which is a different
// rule and a wrong one: an event where the `self` attendee ACCEPTED and some
// other attendee on the owner's address declined would read as declined, and a
// meeting the account is going to would leave the schedule. Whichever entry is
// the owner's, exactly one answer is theirs.
//
// Only "declined" counts. A tentative answer is an attendance somebody may yet
// make, and "needsAction" is an invitation nobody has read; taking either off
// the schedule would hide a meeting that is still going to happen.
func ownerDeclined(attendees []eventActor, owner string) bool {
	if self, ok := ownerAttendee(attendees, owner); ok {
		return strings.EqualFold(strings.TrimSpace(self.ResponseStatus), "declined")
	}
	return false
}

// ownerAttendee picks the attendee entry belonging to the connected account:
// the one Google marked `self`, else the one on the owner's own address.
func ownerAttendee(attendees []eventActor, owner string) (eventActor, bool) {
	for _, a := range attendees {
		if a.Self {
			return a, true
		}
	}
	ownerAddress := strings.ToLower(strings.TrimSpace(owner))
	if ownerAddress == "" {
		return eventActor{}, false
	}
	for _, a := range attendees {
		if strings.EqualFold(strings.TrimSpace(a.Email), ownerAddress) {
			return a, true
		}
	}
	return eventActor{}, false
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
