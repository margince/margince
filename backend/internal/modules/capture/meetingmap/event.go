// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import "time"

// Event is one calendar event as any provider describes it, after that
// provider's own decode. It carries only what the meeting rules read. The raw
// original reaches the Sink verbatim as evidence, so nothing absent here is lost.
type Event struct {
	Private bool
	ID      string
	// ICalUID is the event's identity across providers. ID is only the
	// provider's own number for it, so Google and Graph give one meeting two.
	// The UID names the series, and the occurrence's slot in the series
	// completes the identity (activities.MeetingIdentityKey). Both syncs expand
	// recurrences, so every decoded Event is one occurrence. Empty when the
	// provider states none: a made-up identity would join unrelated meetings.
	ICalUID   string
	Cancelled bool
	Removed   bool
	// OwnerResponse is the connected calendar's answer. Participant rows carry
	// it; only Cancelled cancels the shared meeting.
	OwnerAddress  string
	OwnerResponse string
	Subject       string
	Description   string
	// StartsAt is the event's start as an instant. Zero is an unreadable start,
	// which the Sink stamps with capture time. A decoder that cannot read a
	// start leaves it zero rather than guessing.
	EndsAt   time.Time
	StartsAt time.Time
	// AllDay says the provider stated a date, not a time: StartsAt is then the
	// AllDayStart noon anchor rather than a moment anybody scheduled.
	AllDay bool
	// SeriesSlot is the start the series gave this occurrence, which a move
	// does not change. Zero for a one-off or where the provider states none.
	SeriesSlot time.Time
	Organizer  Actor
	Attendees  []Actor
	// PartiesHidden: the provider withheld guests from this copy of the event.
	PartiesHidden bool
	// GoogleMeet says the event's conference is a Google Meet call.
	GoogleMeet bool
	// Documents are the files the provider lists as attached to the event.
	Documents []Document
}

// Actor is one organizer or attendee. Room marks a booked room or device.
// Response is this attendee's answer as a connector.Response constant, or ""
// when the provider stated none.
type Actor struct {
	Email string
	// Name is what the invitation called this party, or "" when it named
	// nobody. An attendee-only contact gets no other full name.
	Name     string
	Response string
	Room     bool
}
