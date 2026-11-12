// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
)

func calendarEventOf(t *testing.T, raw []byte) *calendarevent.Event {
	t.Helper()
	msg, err := Parse(raw, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("gmail", nil).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("an email record carries ActivityFields")
	}
	return fields.CalendarEvent
}

func at(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// The shape Google Calendar sends for "Updated invitation": an IANA zone, a
// raised SEQUENCE, a folded description and a reminder nested in the event.
func TestAGoogleUpdateNamesItsMeeting(t *testing.T) {
	t.Parallel()
	event := calendarEventOf(t, declineFrom(
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR", "PRODID:-//Google Inc//Google Calendar 70.9054//EN", "METHOD:REQUEST",
		"BEGIN:VEVENT",
		"DTSTART;TZID=Europe/Berlin:20261114T150000",
		"DTEND;TZID=Europe/Berlin:20261114T153000",
		"ORGANIZER;CN=Anna Weber:mailto:Anna.Weber@partner.example",
		"UID:7kq2abcdef@google.com",
		"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;CN=Anna Weber;X-NUM-GUESTS=0:mailto:anna.weber@partner.example",
		"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;CN=owner@myco.com;X-NUM-GUESTS=0:mailto:owner@myco.com",
		"DESCRIPTION:Agenda: pricing\\, rollout",
		" and next steps",
		"LOCATION:Office Berlin\\, Room 4",
		"SEQUENCE:2",
		"STATUS:CONFIRMED",
		"SUMMARY:Pricing review",
		"BEGIN:VALARM", "ACTION:DISPLAY", "TRIGGER:-P0DT0H10M0S", "DESCRIPTION:Reminder", "END:VALARM",
		"END:VEVENT", "END:VCALENDAR",
	))
	if event == nil {
		t.Fatal("no event read from the invitation")
	}
	if event.Sequence != 2 || event.Title != "Pricing review" || event.Location != "Office Berlin, Room 4" {
		t.Errorf("event = %+v, want sequence 2, the summary and the unescaped location", event)
	}
	if event.Start == nil || !event.Start.Equal(at(t, "2026-11-14T14:00:00Z")) {
		t.Errorf("start = %v, want 15:00 Berlin as 14:00 UTC", event.Start)
	}
	if event.End == nil || !event.End.Equal(at(t, "2026-11-14T14:30:00Z")) {
		t.Errorf("end = %v, want 14:30 UTC", event.End)
	}
	if event.Organizer != "anna.weber@partner.example" || len(event.Attendees) != 2 {
		t.Fatalf("organizer %q, attendees %+v", event.Organizer, event.Attendees)
	}
	if guest := event.Attendees[0]; guest.Name != "Anna Weber" || guest.Answer != calendarevent.AnswerAccepted {
		t.Errorf("first guest = %+v, want Anna Weber, accepted", guest)
	}
	if guest := event.Attendees[1]; guest.Email != "owner@myco.com" || guest.Answer != "" {
		t.Errorf("second guest = %+v, want owner@myco.com with no answer yet", guest)
	}
}

// Outlook names a Windows zone no zone database holds and states its rules in
// a VTIMEZONE. A quoted name may hold a comma and a colon.
func TestAnOutlookUpdateIsReadInItsStatedZone(t *testing.T) {
	t.Parallel()
	outlook := func(start string) []string {
		return []string{
			"Content-Type: text/calendar; charset=\"utf-8\"; method=REQUEST",
			"",
			"BEGIN:VCALENDAR", "METHOD:REQUEST", "PRODID:Microsoft Exchange Server 2010", "VERSION:2.0",
			"BEGIN:VTIMEZONE", "TZID:W. Europe Standard Time",
			"BEGIN:STANDARD", "DTSTART:16010101T030000", "TZOFFSETFROM:+0200", "TZOFFSETTO:+0100",
			"RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=-1SU;BYMONTH=10", "END:STANDARD",
			"BEGIN:DAYLIGHT", "DTSTART:16010101T020000", "TZOFFSETFROM:+0100", "TZOFFSETTO:+0200",
			"RRULE:FREQ=YEARLY;INTERVAL=1;BYDAY=-1SU;BYMONTH=3", "END:DAYLIGHT",
			"END:VTIMEZONE",
			"BEGIN:VEVENT",
			`ORGANIZER;CN="Weber, Anna: Sales":mailto:anna.weber@partner.example`,
			`ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE;CN="Owner, Me":mailto:owner@myco.com`,
			"SUMMARY;LANGUAGE=de-DE:Angebot besprechen",
			"DTSTART;TZID=W. Europe Standard Time:" + start,
			"DTEND;TZID=W. Europe Standard Time:" + start[:9] + "110000",
			"UID:040000008200E00074C5B7101A82E00800000000",
			"SEQUENCE:1",
			"LOCATION;LANGUAGE=de-DE:Microsoft Teams-Besprechung",
			"STATUS:CONFIRMED",
			"END:VEVENT", "END:VCALENDAR",
		}
	}
	for name, tc := range map[string]struct{ start, want string }{
		"winter, standard time":                          {"20261202T100000", "2026-12-02T09:00:00Z"},
		"summer, daylight time":                          {"20260702T100000", "2026-07-02T08:00:00Z"},
		"the day daylight time starts, after the change": {"20260329T100000", "2026-03-29T08:00:00Z"},
		"the day before daylight time starts":            {"20260328T100000", "2026-03-28T09:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			event := calendarEventOf(t, declineFrom(outlook(tc.start)...))
			if event == nil || event.Start == nil {
				t.Fatalf("event %+v names no start", event)
			}
			if !event.Start.Equal(at(t, tc.want)) {
				t.Errorf("start = %v, want %s", event.Start.UTC(), tc.want)
			}
			if event.Title != "Angebot besprechen" || len(event.Attendees) != 1 || event.Attendees[0].Name != "Owner, Me" {
				t.Errorf("event = %+v, want the title and the quoted guest name whole", event)
			}
		})
	}
}

// A zone neither the database nor the calendar states names no instant: no
// time is stored rather than a wrong one. The rest of the event still is.
func TestAnUnknownZoneStoresNoTime(t *testing.T) {
	t.Parallel()
	event := calendarEventOf(t, declineFrom(
		"Content-Type: text/calendar; method=REQUEST",
		"",
		"BEGIN:VCALENDAR", "METHOD:REQUEST",
		"BEGIN:VEVENT", "UID:x@example.com", "SUMMARY:Demo",
		"DTSTART;TZID=Mars Standard Time:20261114T150000",
		"END:VEVENT", "END:VCALENDAR",
	))
	if event == nil || event.Start != nil || event.Title != "Demo" {
		t.Errorf("event = %+v, want the title and no start", event)
	}
}

func TestACancellationAndAnAllDayEventAreRead(t *testing.T) {
	t.Parallel()
	event := calendarEventOf(t, declineFrom(
		"Content-Type: text/calendar; method=CANCEL",
		"",
		"BEGIN:VCALENDAR", "METHOD:CANCEL",
		"BEGIN:VEVENT", "UID:offsite@example.com", "SUMMARY:Offsite",
		"DTSTART;VALUE=DATE:20261120", "DTEND;VALUE=DATE:20261121",
		"STATUS:CANCELLED", "SEQUENCE:3",
		"RECURRENCE-ID;VALUE=DATE:20261120",
		"END:VEVENT", "END:VCALENDAR",
	))
	if event == nil || !event.Canceled || !event.AllDay || event.Sequence != 3 {
		t.Fatalf("event = %+v, want a cancelled all-day event at sequence 3", event)
	}
	if !event.Start.Equal(at(t, "2026-11-20T00:00:00Z")) || event.Occurrence != "20261120" {
		t.Errorf("start %v, occurrence %q, want the date both ways", event.Start, event.Occurrence)
	}
}

// One meeting of a series is named by its original start in UTC. So two sends
// about it meet on the same key, however the calendar spelled it.
func TestAnOccurrenceIsNamedByItsOriginalStart(t *testing.T) {
	t.Parallel()
	event := calendarEventOf(t, declineFrom(
		"Content-Type: text/calendar; method=REQUEST",
		"",
		"BEGIN:VCALENDAR", "METHOD:REQUEST",
		"BEGIN:VEVENT", "UID:weekly@example.com",
		"RECURRENCE-ID;TZID=Europe/Berlin:20261118T090000",
		"DTSTART;TZID=Europe/Berlin:20261119T090000",
		"END:VEVENT", "END:VCALENDAR",
	))
	if event == nil || event.Occurrence != "2026-11-18T08:00:00Z" {
		t.Errorf("event = %+v, want the occurrence as 08:00 UTC", event)
	}
}

// application/ics is the same payload under another declared type.
func TestAnApplicationICSAttachmentIsACalendarPart(t *testing.T) {
	t.Parallel()
	raw := declineFrom(
		"Content-Type: application/ics; name=\"invite.ics\"",
		"Content-Disposition: attachment; filename=\"invite.ics\"",
		"",
		"BEGIN:VCALENDAR", "METHOD:REQUEST",
		"BEGIN:VEVENT", "UID:ics@example.com", "SUMMARY:Kickoff", "SEQUENCE:0", "END:VEVENT",
		"END:VCALENDAR",
	)
	if got := calendarMethodOf(t, raw); got != "REQUEST" {
		t.Errorf("method = %q, want REQUEST", got)
	}
	if event := calendarEventOf(t, raw); event == nil || event.Title != "Kickoff" {
		t.Errorf("event = %+v, want Kickoff", event)
	}
}

// A guest list past the bound is cut and says so, so no comparison reads a
// guest past the cut as removed.
func TestALongGuestListIsCutAndSaysSo(t *testing.T) {
	t.Parallel()
	part := []string{
		"Content-Type: text/calendar; method=REQUEST", "",
		"BEGIN:VCALENDAR", "METHOD:REQUEST", "BEGIN:VEVENT", "UID:allhands@example.com",
	}
	for i := range calendarevent.MaxAttendees + 5 {
		part = append(part, "ATTENDEE:mailto:guest"+string(rune('a'+i%26))+string(rune('a'+i/26))+"@example.com")
	}
	part = append(part, "END:VEVENT", "END:VCALENDAR")
	event := calendarEventOf(t, declineFrom(part...))
	if event == nil || len(event.Attendees) != calendarevent.MaxAttendees || !event.AttendeesCut {
		t.Errorf("event holds %d guests, cut %v; want %d and cut", len(event.Attendees), event.AttendeesCut, calendarevent.MaxAttendees)
	}
}
