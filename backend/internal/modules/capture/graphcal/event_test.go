// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const owner = "rep@myco.com"

// eventJSON builds a raw Graph event resource for the mapper fixtures. The
// organizer is explicit so a fixture can model an externally-organized meeting
// the owner merely attends, not only owner-organized ones.
func eventJSON(t *testing.T, id, subject, start, tz, organizer string, attendees ...string) []byte {
	t.Helper()
	return eventJSONWith(t, map[string]any{
		"id":        id,
		"subject":   subject,
		"start":     map[string]string{"dateTime": start, "timeZone": tz},
		"organizer": actorJSON(organizer),
		"attendees": attendeesJSON(attendees...),
	})
}

func eventJSONWith(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return b
}

func actorJSON(address string) map[string]any {
	return map[string]any{"emailAddress": map[string]string{"address": address}}
}

func attendeesJSON(addresses ...string) []map[string]any {
	out := make([]map[string]any, 0, len(addresses))
	for _, a := range addresses {
		out = append(out, map[string]any{
			"type": "required", "emailAddress": map[string]string{"address": a},
		})
	}
	return out
}

func TestParseEventMapsMeetingActivity(t *testing.T) {
	raw := eventJSON(t, "evt-1", "Kickoff", "2026-07-16T12:00:00.0000000", "Europe/Berlin",
		owner, owner, "client@acme.com")
	m, err := classifyRaw(raw, owner)
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	if reason, skip := m.SkipReason(); skip {
		t.Fatalf("a meeting with an external attendee must not skip, got %q", reason)
	}
	rec := m.ToRecord(connectorName, raw)
	if rec.EntityType != datasource.EntityActivity {
		t.Errorf("EntityType = %v, want activity", rec.EntityType)
	}
	if rec.NaturalKey.SourceSystem != connectorName || rec.NaturalKey.SourceID != "evt-1" {
		t.Errorf("NaturalKey = %+v, want graphcal/evt-1", rec.NaturalKey)
	}
	if rec.Source != "graphcal:evt-1" || rec.CapturedBy != "connector:graphcal" {
		t.Errorf("provenance = (%q,%q), want (graphcal:evt-1, connector:graphcal)", rec.Source, rec.CapturedBy)
	}
	fields := activityFields(t, rec)
	if fields.Kind != "meeting" || fields.Subject != "Kickoff" {
		t.Errorf("Kind/Subject = (%q,%q), want (meeting, Kickoff)", fields.Kind, fields.Subject)
	}
	if fields.Direction != "" {
		t.Errorf("Direction = %q, want empty (a meeting is not directional)", fields.Direction)
	}
	// Graph states a wall clock plus the zone it is stated in — never an
	// offset — so noon in Berlin is 10:00 UTC in July.
	want := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	if !fields.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt = %v, want %v (the stated zone applied)", fields.OccurredAt, want)
	}
}

// A zone name Go cannot resolve must not become a wrong instant: Microsoft
// still emits Windows zone ids ("W. Europe Standard Time") on some calendars,
// and reading one as UTC would file every such meeting hours from where it was.
func TestAnUnresolvableZoneYieldsNoStartRatherThanAWrongOne(t *testing.T) {
	raw := eventJSON(t, "evt-tz", "Onsite", "2026-07-16T12:00:00.0000000",
		"W. Europe Standard Time", owner, "client@acme.com")
	if got := activityFields(t, mustRecord(t, raw)).OccurredAt; !got.IsZero() {
		t.Fatalf("OccurredAt = %v, want the zero time so the Sink stamps capture time honestly", got)
	}
}

func TestAllDayEventIsAnchoredAtNoon(t *testing.T) {
	raw := eventJSONWith(t, map[string]any{
		"id": "evt-allday", "subject": "Onsite", "isAllDay": true,
		"start":     map[string]string{"dateTime": "2026-07-16T00:00:00.0000000", "timeZone": "UTC"},
		"organizer": actorJSON(owner),
		"attendees": attendeesJSON("client@acme.com"),
	})
	// Anchored at noon so the date survives the ±12h of real-world offsets;
	// midnight would slip to the previous day for any zone west of UTC.
	want := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	if got := activityFields(t, mustRecord(t, raw)).OccurredAt; !got.Equal(want) {
		t.Errorf("all-day OccurredAt = %v, want %v (noon UTC)", got, want)
	}
}

// A delta round reports a deletion as a tombstone carrying little but the id.
// It is a cancellation to a calendar, and the shared rules drop those.
func TestARemovedEventIsDropped(t *testing.T) {
	raw := eventJSONWith(t, map[string]any{
		"id": "evt-gone", "@removed": map[string]string{"reason": "deleted"},
	})
	reason, skip := mustParse(t, raw).SkipReason()
	if !skip || reason != "cancelled" {
		t.Fatalf("a removed event: got (%q, skip=%v), want it dropped as cancelled", reason, skip)
	}
}

func TestACancelledEventIsDropped(t *testing.T) {
	raw := eventJSONWith(t, map[string]any{
		"id": "evt-cx", "subject": "Cancelled call", "isCancelled": true,
		"start":     map[string]string{"dateTime": "2026-07-16T09:00:00.0000000", "timeZone": "UTC"},
		"organizer": actorJSON(owner),
		"attendees": attendeesJSON("client@acme.com"),
	})
	if reason, skip := mustParse(t, raw).SkipReason(); !skip || reason != "cancelled" {
		t.Fatalf("cancelled event: got (%q, skip=%v), want cancelled skip", reason, skip)
	}
}

// A booked room is not a guest. Microsoft says so with the attendee's type, and
// counting it would make every all-colleague meeting held in a room look like
// it had an outside party.
func TestABookedRoomIsNotAGuest(t *testing.T) {
	raw := eventJSONWith(t, map[string]any{
		"id": "evt-room", "subject": "Standup",
		"start":     map[string]string{"dateTime": "2026-07-16T09:00:00.0000000", "timeZone": "UTC"},
		"organizer": actorJSON(owner),
		"attendees": []map[string]any{
			{"type": "required", "emailAddress": map[string]string{"address": "peer@myco.com"}},
			{"type": "resource", "emailAddress": map[string]string{"address": "boardroom@myco.com"}},
		},
	})
	m := mustParse(t, raw)
	reason, skip := m.SkipReason()
	if !skip || reason != "no party outside the owner's domain" {
		t.Fatalf("got (%q, skip=%v), want the room ignored and the meeting dropped as internal", reason, skip)
	}
	for _, p := range m.Participants() {
		if p.Email == "boardroom@myco.com" {
			t.Error("a booked room reached the participant rows")
		}
	}
}

func TestAMeetingWithAnExternalGuestReportsEveryParty(t *testing.T) {
	raw := eventJSON(t, "evt-ext", "Demo", "2026-07-16T09:00:00.0000000", "UTC",
		"host@acme.com", owner)
	rec := mustRecord(t, raw)
	want := []string{"host@acme.com", owner}
	if got := rec.Addresses; !slices.Equal(got, want) {
		t.Errorf("Addresses = %v, want %v", got, want)
	}
	if body := activityFields(t, rec).Body; !strings.Contains(body, "host@acme.com") {
		t.Errorf("body = %q, want the external organizer named in it", body)
	}
}

func TestParseEventRejectsMalformedJSON(t *testing.T) {
	if _, err := classifyRaw([]byte("}not json{"), owner); err == nil {
		t.Fatal("decodeEvent must reject malformed event bytes")
	}
}

func TestParticipantsOfReadsAStoredEvent(t *testing.T) {
	raw := eventJSON(t, "evt-p", "Review", "2026-07-16T09:00:00.0000000", "UTC",
		"host@acme.com", owner, "second@acme.com")
	parts, err := ParticipantsOf(raw, owner)
	if err != nil {
		t.Fatalf("ParticipantsOf: %v", err)
	}
	roles := map[string]string{}
	for _, p := range parts {
		roles[p.Email] = p.Role
	}
	if roles["host@acme.com"] != connector.ParticipantRoleOrganizer {
		t.Errorf("organizer role = %q", roles["host@acme.com"])
	}
	if roles["second@acme.com"] != connector.ParticipantRoleAttendee {
		t.Errorf("attendee role = %q", roles["second@acme.com"])
	}
	if _, ok := roles[owner]; ok {
		t.Error("the owner must not appear among the participant rows — capture stamps them from the connection")
	}
}

// --- helpers -------------------------------------------------------------

func mustParse(t *testing.T, raw []byte) meetingmap.Meeting {
	t.Helper()
	m, err := classifyRaw(raw, owner)
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	return m
}

func mustRecord(t *testing.T, raw []byte) connector.NormalizedRecord {
	t.Helper()
	return mustParse(t, raw).ToRecord(connectorName, raw)
}

func activityFields(t *testing.T, rec connector.NormalizedRecord) capture.ActivityFields {
	t.Helper()
	fields, ok := rec.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatalf("Fields is %T, want capture.ActivityFields", rec.Fields)
	}
	return fields
}

// classifyRaw is the connector's own path in one call — decode this vendor's
// bytes, then apply the shared meeting rules — so a fixture asserts on the
// result rather than on either half.
func classifyRaw(raw []byte, owner string) (meetingmap.Meeting, error) {
	ev, err := decodeEvent(raw)
	if err != nil {
		return meetingmap.Meeting{}, err
	}
	return meetingmap.Classify(ev, owner), nil
}
