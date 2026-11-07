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
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rawEvent is the subset of a Google Calendar v3 event resource this decode
// reads. Unknown fields are ignored — the raw original is stored verbatim as
// evidence (memory-first), so nothing is lost by reading only what we use.
type rawEvent struct {
	Visibility string `json:"visibility"`
	ID         string `json:"id"`
	// ICalUID is the event's cross-provider identity, and `id` is not: `id` is
	// Google's own numbering, so the same meeting read from Google and from
	// Graph carries two ids and would dedupe as two meetings.
	//
	// The tag is Google's own spelling. Microsoft writes `iCalUId`, and both
	// decode either payload because encoding/json matches field names case
	// INSENSITIVELY — so the two spellings are documentation of each vendor's
	// wire format, not a correctness boundary, and no test can tell them apart.
	//
	// With singleEvents=true (client.go) Google returns it per OCCURRENCE, and
	// every occurrence of one series carries the same value — so it names the
	// series and the occurrence's start names the meeting within it.
	ICalUID     string        `json:"iCalUID"` //nolint:tagliatelle // Google's wire format
	Status      string        `json:"status"`  // "confirmed" | "tentative" | calendarCanceled
	Summary     string        `json:"summary"`
	Description string        `json:"description"`
	End         eventDateTime `json:"end"`
	Start       eventDateTime `json:"start"`
	// OriginalStart is an occurrence's slot in its series, which a move does
	// not change. Google states it only on an instance of a recurring event.
	OriginalStart eventDateTime `json:"originalStartTime"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Organizer     eventActor    `json:"organizer"`
	Attendees     []eventActor  `json:"attendees"`
	// Google sets attendeesOmitted when it cut the list short. A guest list
	// hidden from guests (guestsCanSeeOtherGuests false, default true) is
	// withheld from every copy but the organizer's.
	AttendeesOmitted        bool  `json:"attendeesOmitted"`        //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	GuestsCanSeeOtherGuests *bool `json:"guestsCanSeeOtherGuests"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	// Attachments are the Drive files attached to the event. Google attaches
	// the notes and transcript it writes for a Meet call here.
	Attachments []eventAttachment `json:"attachments"`
	// Conference is the call the event carries, read only for its kind.
	Conference eventConference `json:"conferenceData"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

// eventAttachment is one Drive file attached to an event.
type eventAttachment struct {
	FileID   string `json:"fileId"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Title    string `json:"title"`
	MimeType string `json:"mimeType"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

// eventConference is the part of an event's conferenceData that says which
// kind of call it is.
type eventConference struct {
	Solution struct {
		Key struct {
			Type string `json:"type"`
		} `json:"key"`
	} `json:"conferenceSolution"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
}

// googleMeetType is the conference type Google states for a Meet call.
const googleMeetType = "hangoutsMeet"

// partiesHidden reports whether this copy of the event may leave guests out.
func (ev rawEvent) partiesHidden() bool {
	guestListHidden := ev.GuestsCanSeeOtherGuests != nil && !*ev.GuestsCanSeeOtherGuests
	return ev.AttendeesOmitted || (guestListHidden && !ev.Organizer.Self)
}

// eventDateTime is a calendar timestamp: dateTime (RFC3339) for a timed event,
// or date (YYYY-MM-DD) for an all-day one.
type eventDateTime struct {
	DateTime string `json:"dateTime"` //nolint:tagliatelle // Google's wire format (camelCase); must match to decode
	Date     string `json:"date"`
}

// eventActor is one organizer/attendee: the email the mapping resolves the
// counterparty by, Google's resource flag marking a booked room or device
// rather than a contact, and the name the organizer typed for them.
//
// The display name is the only place an attendee is named in full. A contact
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

// isRoom reports whether this attendee is a booked room or device rather than a
// contact. A room is not a party to a meeting: it cannot be a counterparty, it
// cannot be answered, and — the case that matters — it is on no workspace's own
// domain, so counting it would make every all-colleague meeting held in a
// booked room look like it had an outside guest.
func (a eventActor) isRoom() bool {
	if a.Resource {
		return true
	}
	dom := domainOf(a.Email)
	return values.IsCalendarResourceDomain(owndomain.Fold(dom))
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
	documents := make([]meetingmap.Document, 0, len(ev.Attachments))
	for _, a := range ev.Attachments {
		documents = append(documents, meetingmap.Document{FileID: a.FileID, Title: a.Title, MimeType: a.MimeType})
	}
	attendees := make([]meetingmap.Actor, 0, len(ev.Attendees))
	for _, a := range ev.Attendees {
		attendees = append(attendees, meetingmap.Actor{
			Email: a.Email, Name: a.DisplayName, Room: a.isRoom(), Response: guestResponse(a.ResponseStatus),
		})
	}
	return meetingmap.Event{
		ID:            ev.ID,
		Private:       ev.Visibility != "" && ev.Visibility != "default" && ev.Visibility != "public",
		ICalUID:       strings.TrimSpace(ev.ICalUID),
		Cancelled:     strings.EqualFold(strings.TrimSpace(ev.Status), calendarCanceled),
		OwnerResponse: ownerResponse(ev.Attendees, owner),
		Subject:       ev.Summary,
		Description:   ev.Description,
		StartsAt:      parseStart(ev.Start),
		EndsAt:        parseStart(ev.End),
		AllDay:        isAllDay(ev.Start),
		SeriesSlot:    parseStart(ev.OriginalStart),
		Organizer:     meetingmap.Actor{Email: ev.Organizer.Email, Name: ev.Organizer.DisplayName},
		Attendees:     attendees,
		PartiesHidden: ev.partiesHidden(),
		GoogleMeet:    ev.Conference.Solution.Key.Type == googleMeetType,
		Documents:     documents,
	}
}

// guestResponses maps Google's responseStatus onto the connector's answers. A
// value Google adds later reads as no stated answer rather than a wrong one.
var guestResponses = map[string]string{
	"accepted":    connector.ResponseAccepted,
	"declined":    connector.ResponseDeclined,
	"tentative":   connector.ResponseTentative,
	"needsaction": connector.ResponseNeedsAction,
}

func guestResponse(status string) string {
	return guestResponses[strings.ToLower(strings.TrimSpace(status))]
}

// ownerResponse prefers Google's self entry, so aliases and group invitations
// use the connected calendar's answer rather than an address match's answer.
func ownerResponse(attendees []eventActor, owner string) string {
	if self, ok := ownerAttendee(attendees, owner); ok {
		return guestResponse(self.ResponseStatus)
	}
	return ""
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
func ParticipantsOf(raw []byte, owner string) (connector.Parties, error) {
	return meetingmap.ParticipantsOf(raw, owner, decodeEvent)
}

// SeriesIdentityOf reads the identity of one stored occurrence of a series
// (meetingmap.SeriesIdentityOf).
func SeriesIdentityOf(raw []byte) (connector.CrossDoorIdentity, bool, error) {
	return meetingmap.SeriesIdentityOf(raw, decodeEvent)
}

// RecordOf maps one stored event resource to the record live capture writes
// for owner, refusing with connector.ErrSkip what live capture drops.
func RecordOf(raw []byte, owner string) (connector.NormalizedRecord, error) {
	recs, err := meetingmap.NormalizeOne(raw, owner, connectorName, decodeEvent)
	if err != nil {
		return connector.NormalizedRecord{}, err
	}
	return recs[0], nil
}

// SelfAddress is the address of the calendar a stored event was read from:
// the entry Google marked `self`, or "" when none is.
func SelfAddress(raw []byte) (string, error) {
	var ev rawEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return "", fmt.Errorf("gcal: parsing calendar event: %w", err)
	}
	if ev.Organizer.Self {
		return strings.ToLower(strings.TrimSpace(ev.Organizer.Email)), nil
	}
	for _, a := range ev.Attendees {
		if a.Self {
			return strings.ToLower(strings.TrimSpace(a.Email)), nil
		}
	}
	return "", nil
}

// SettlementOf answers what one stored event resource settles as, for the
// backfill that closes meetings captured before the RSVP was read.
func SettlementOf(raw []byte, owner string) (meetingmap.Settlement, error) {
	return meetingmap.SettlementOf(raw, owner, decodeEvent)
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

// isAllDay reports that Google stated the start as a date with no time — the
// shape parseStart anchors at noon.
func isAllDay(start eventDateTime) bool {
	if strings.TrimSpace(start.DateTime) != "" {
		return false
	}
	_, ok := meetingmap.AllDayStart(start.Date)
	return ok
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
