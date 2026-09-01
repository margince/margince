// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

// The rules here are shared by every calendar connector, so they are proved
// once here rather than twice through each vendor's decode.

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const owner = "rep@myco.com"

// Unknown is not internal, for the organizer as much as for an attendee. An
// address that will not parse is the one case where reading it the other way
// silently drops a real multi-party meeting.
func TestAnOrganizerWithNoParseableDomainCountsAsExternal(t *testing.T) {
	m := Classify(Event{
		ID: "evt-odd", Subject: "Intro",
		Organizer: Actor{Email: "weird-address"},
		Attendees: []Actor{{Email: owner}},
	}, owner)
	if reason, skip := m.SkipReason(); skip {
		t.Fatalf("dropped as %q — a malformed organizer address is not proof the meeting was internal", reason)
	}
}

// An event NAMING no organizer is not thereby external: absence is absence.
func TestAnEventWithNoOrganizerIsStillJudgedOnItsParties(t *testing.T) {
	solo := Classify(Event{
		ID: "evt-solo", Subject: "Focus time",
		Attendees: []Actor{{Email: owner}},
	}, owner)
	if _, skip := solo.SkipReason(); !skip {
		t.Fatal("a block naming nobody but the owner must skip")
	}
	internal := Classify(Event{
		ID: "evt-standup", Subject: "Standup",
		Attendees: []Actor{{Email: owner}, {Email: "peer@myco.com"}},
	}, owner)
	if reason, skip := internal.SkipReason(); !skip || reason != "no party outside the owner's domain" {
		t.Fatalf("got (%q, skip=%v), want the owner-domain floor to drop it", reason, skip)
	}
}

// A booked room is not a guest. It sits on no workspace's own domain, so
// counting it would make every all-colleague meeting held in one look like it
// had an outside party.
func TestABookedRoomIsNeitherAPartyNorAParticipant(t *testing.T) {
	m := Classify(Event{
		ID: "evt-room", Subject: "Standup",
		Organizer: Actor{Email: owner},
		Attendees: []Actor{
			{Email: "peer@myco.com"},
			{Email: "boardroom@myco.com", Room: true},
		},
	}, owner)
	if reason, skip := m.SkipReason(); !skip || reason != "no party outside the owner's domain" {
		t.Fatalf("got (%q, skip=%v), want the room ignored and the meeting dropped as internal", reason, skip)
	}
	for _, p := range m.Participants() {
		if strings.HasPrefix(p.Email, "boardroom@") {
			t.Error("a booked room reached the participant rows")
		}
	}
}

func TestACancelledEventAndOneWithNoIDAreDropped(t *testing.T) {
	external := []Actor{{Email: "client@acme.test"}}
	for name, ev := range map[string]Event{
		"cancelled": {ID: "evt-1", Cancelled: true, Organizer: Actor{Email: owner}, Attendees: external},
		"no id":     {Organizer: Actor{Email: owner}, Attendees: external},
	} {
		t.Run(name, func(t *testing.T) {
			if _, skip := Classify(ev, owner).SkipReason(); !skip {
				t.Fatalf("a %s event must be dropped", name)
			}
		})
	}
}

// The owner is excluded from the participant rows because capture stamps them
// from the CONNECTION, which carries their user_id — the column the interaction
// graph joins on. A row built from this header would carry only an address.
func TestTheOwnerIsNotAParticipantAndOrganizerOutranksAttending(t *testing.T) {
	m := Classify(Event{
		ID: "evt-2", Subject: "Review",
		Organizer: Actor{Email: "host@acme.test"},
		// The organizer attends too, which is the common case.
		Attendees: []Actor{{Email: owner}, {Email: "host@acme.test"}, {Email: "second@acme.test"}},
	}, owner)

	roles := map[string]string{}
	for _, p := range m.Participants() {
		roles[p.Email] = p.Role
	}
	if _, ok := roles[owner]; ok {
		t.Error("the owner appears among the participant rows")
	}
	if roles["host@acme.test"] != connector.ParticipantRoleOrganizer {
		t.Errorf("host role = %q, want organizer to outrank attending", roles["host@acme.test"])
	}
	if roles["second@acme.test"] != connector.ParticipantRoleAttendee {
		t.Errorf("second role = %q, want attendee", roles["second@acme.test"])
	}
}

// The stored record is what the timeline reads and what the writer judges
// against the workspace's registered domains.
func TestTheRecordCarriesEveryPartyAndItsConnectorsProvenance(t *testing.T) {
	m := Classify(Event{
		ID: "evt-3", Subject: "Demo", Description: "the agenda",
		StartsAt:  time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
		Organizer: Actor{Email: "host@acme.test"},
		Attendees: []Actor{{Email: owner}},
	}, owner)

	rec := m.ToRecord("graphcal", []byte(`{"id":"evt-3"}`))
	if rec.NaturalKey.SourceSystem != "graphcal" || rec.NaturalKey.SourceID != "evt-3" {
		t.Errorf("NaturalKey = %+v", rec.NaturalKey)
	}
	if rec.CapturedBy != "connector:graphcal" {
		t.Errorf("CapturedBy = %q", rec.CapturedBy)
	}
	// The owner IS among the addresses even though they are not a participant:
	// the internal decision is taken against the workspace's registered
	// domains, and a workspace that has registered none must not have one
	// asserted for it here.
	want := map[string]bool{"host@acme.test": true, owner: true}
	for _, a := range rec.Addresses {
		delete(want, a)
	}
	if len(want) != 0 {
		t.Errorf("Addresses = %v, missing %v", rec.Addresses, want)
	}
	fields := activityFields(t, rec)
	if fields.Kind != "meeting" || fields.Direction != "" {
		t.Errorf("Kind/Direction = (%q,%q), want a non-directional meeting", fields.Kind, fields.Direction)
	}
	if !strings.Contains(fields.Body, "host@acme.test") || !strings.Contains(fields.Body, "the agenda") {
		t.Errorf("body = %q, want the organizer and the description folded in", fields.Body)
	}
}

func TestTheBodyIsBoundedAndEndsOnARuneBoundary(t *testing.T) {
	m := Classify(Event{
		ID: "evt-4", Subject: "Big",
		Description: strings.Repeat("ä", MaxBodyLen),
		Organizer:   Actor{Email: owner},
		Attendees:   []Actor{{Email: "client@acme.test"}},
	}, owner)
	body := activityFields(t, m.ToRecord("graphcal", nil)).Body
	if len(body) > MaxBodyLen+len("…") {
		t.Errorf("body of %d bytes exceeds the cap %d", len(body), MaxBodyLen)
	}
	if !strings.HasSuffix(body, "…") {
		t.Error("a truncated body must end with an ellipsis")
	}
	if strings.ContainsRune(body, '�') {
		t.Error("the truncation broke a multi-byte rune")
	}
}

// An all-day date is anchored at noon so it keeps its calendar date across the
// ±12h of real offsets; midnight would slip to the previous day west of UTC.
func TestAnAllDayDateIsAnchoredAtNoon(t *testing.T) {
	got, ok := AllDayStart("2026-07-16")
	if !ok {
		t.Fatal("AllDayStart refused a well-formed date")
	}
	if want := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("AllDayStart = %v, want %v", got, want)
	}
	if _, ok := AllDayStart("not a date"); ok {
		t.Error("AllDayStart accepted something that is not a date")
	}
}

func activityFields(t *testing.T, rec connector.NormalizedRecord) capture.ActivityFields {
	t.Helper()
	fields, ok := rec.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatalf("Fields is %T, want capture.ActivityFields", rec.Fields)
	}
	return fields
}
