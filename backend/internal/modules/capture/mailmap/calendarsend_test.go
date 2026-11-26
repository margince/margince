// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// googleGuestCopy is one guest's copy of an invitation Google Calendar sent
// for the organizer. It has a Message-ID of its own and the guest in To. Its
// calendar part is the one every guest's copy of that send carries.
func googleGuestCopy(messageID, guest, method string, sequence string) []byte {
	return crlf(
		"Sender: Google Calendar <calendar-notification@google.com>",
		"Message-ID: <"+messageID+"@google.com>",
		"From: Lars Organizer <owner@myco.com>",
		"To: "+guest,
		"Subject: Invitation: Weekly sync @ Fri 5 Jun 2026 11:15am ("+guest+")",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Reply for "+guest,
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method="+method,
		"",
		"BEGIN:VCALENDAR",
		"METHOD:"+method,
		"BEGIN:VEVENT",
		"UID:5c32ehv2a2j2aso23n324ua1ke@google.com",
		"SEQUENCE:"+sequence,
		"SUMMARY:Weekly sync",
		"DTSTART:20260605T091500Z",
		"DTEND:20260605T094500Z",
		"ORGANIZER:mailto:owner@myco.com",
		"ATTENDEE;PARTSTAT=ACCEPTED:mailto:owner@myco.com",
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ana@partner.example",
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ben@partner.example",
		"END:VEVENT",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	)
}

func calendarSendOf(t *testing.T, raw []byte, owner string) string {
	t.Helper()
	msg, err := Parse(raw, owner)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("gmail", raw).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("a mail record carries ActivityFields")
	}
	return fields.CalendarSend
}

func TestEveryGuestsCopyOfOneSendNamesThatSend(t *testing.T) {
	t.Parallel()
	ana := calendarSendOf(t, googleGuestCopy("calendar-1", "ana@partner.example", "REQUEST", "0"), "owner@myco.com")
	ben := calendarSendOf(t, googleGuestCopy("calendar-2", "ben@partner.example", "REQUEST", "0"), "owner@myco.com")
	if ana == "" {
		t.Fatal("a guest's copy of the organizer's invitation names no send")
	}
	if ana != ben {
		t.Errorf("two guests' copies of one send name %q and %q, want one send", ana, ben)
	}
}

// A later send of the same event is news of its own: an update, or a call-off.
func TestALaterSendOfTheEventIsAnotherSend(t *testing.T) {
	t.Parallel()
	first := calendarSendOf(t, googleGuestCopy("calendar-1", "ana@partner.example", "REQUEST", "0"), "owner@myco.com")
	for name, raw := range map[string][]byte{
		"an update":      googleGuestCopy("calendar-3", "ana@partner.example", "REQUEST", "1"),
		"a cancellation": googleGuestCopy("calendar-4", "ana@partner.example", "CANCEL", "0"),
	} {
		if got := calendarSendOf(t, raw, "owner@myco.com"); got == first || got == "" {
			t.Errorf("%s names send %q, want one of its own beside %q", name, got, first)
		}
	}
}

// Only the organizer's Sent holds a copy per guest. An invitation a seat
// receives is the one mail its organizer sent them.
func TestAReceivedInvitationNamesNoSend(t *testing.T) {
	t.Parallel()
	raw := googleGuestCopy("calendar-5", "ana@partner.example", "REQUEST", "0")
	if got := calendarSendOf(t, raw, "ana@partner.example"); got != "" {
		t.Errorf("a received invitation names send %q, want none", got)
	}
}
