// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// declineFrom builds a contact's answer to an invitation, with the calendar
// part shaped by the caller.
func declineFrom(calendarPart ...string) []byte {
	lines := []string{
		"Message-ID: <decline-1@partner.example>",
		"From: Buyer <buyer@partner.example>",
		"To: owner@myco.com",
		"Subject: Declined: Demo @ Thu 15:00",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Buyer has declined this invitation.",
		"--b1",
	}
	lines = append(lines, calendarPart...)
	lines = append(lines, "--b1--", "")
	return crlf(lines...)
}

func calendarMethodOf(t *testing.T, raw []byte) string {
	t.Helper()
	msg, err := Parse(raw, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("gmail", nil).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("an email record carries ActivityFields")
	}
	if !fields.HasCalendarPart {
		t.Fatal("the calendar part was not recognised at all")
	}
	return fields.CalendarMethod
}

func TestACalendarPartNamesItsMethod(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		part []string
		want string
	}{{
		// Google: an unnamed inline part, the method in its Content-Type.
		name: "parameter on an inline part",
		part: []string{
			"Content-Type: text/calendar; charset=UTF-8; method=REPLY",
			"",
			"BEGIN:VCALENDAR",
			"BEGIN:VEVENT",
			"END:VEVENT",
			"END:VCALENDAR",
		},
		want: "REPLY",
	}, {
		// Outlook: an attached invite.ics, the method only in the body.
		name: "method only in a named text/calendar attachment",
		part: []string{
			`Content-Type: text/calendar; name="invite.ics"`,
			`Content-Disposition: attachment; filename="invite.ics"`,
			"",
			"BEGIN:VCALENDAR",
			"PRODID:-//Microsoft Corporation//Outlook 16.0 MIMEDIR//EN",
			"METHOD:REPLY",
			"END:VCALENDAR",
		},
		want: "REPLY",
	}, {
		name: "folded line and lower case",
		part: []string{
			"Content-Type: text/calendar",
			"",
			"BEGIN:VCALENDAR",
			"METHOD:decline",
			" counter",
			"END:VCALENDAR",
		},
		want: "DECLINECOUNTER",
	}, {
		name: "an unknown method",
		part: []string{
			"Content-Type: text/calendar; method=X-ANSWER",
			"",
			"BEGIN:VCALENDAR",
			"METHOD:X-ANSWER",
			"END:VCALENDAR",
		},
		want: "",
	}, {
		// Somebody attached an event to a mail they wrote: no scheduling act.
		name: "a hand-attached file with no method",
		part: []string{
			`Content-Type: text/calendar; name="party.ics"`,
			`Content-Disposition: attachment; filename="party.ics"`,
			"",
			"BEGIN:VCALENDAR",
			"BEGIN:VEVENT",
			"SUMMARY:Summer party",
			"END:VEVENT",
			"END:VCALENDAR",
		},
		want: "",
	}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := declineFrom(tc.part...)
			if got := calendarMethodOf(t, raw); got != tc.want {
				t.Errorf("method = %q, want %q", got, tc.want)
			}
		})
	}
}

// A sender who wants a request filed as an answer says REPLY somewhere. Every
// disagreement is resolved against them: the request side wins.
func TestACalendarMethodConflictIsResolvedAgainstTheSender(t *testing.T) {
	t.Parallel()
	replyHeaderRequestBody := []string{
		"Content-Type: text/calendar; method=REPLY",
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REQUEST",
		"END:VCALENDAR",
	}
	if got := calendarMethodOf(t, declineFrom(replyHeaderRequestBody...)); got != "REQUEST" {
		t.Errorf("a REPLY header over a REQUEST body stored %q, want REQUEST", got)
	}

	dummyReplyThenRequest := []string{
		`Content-Type: text/calendar; method=REPLY; name="reply.ics"`,
		`Content-Disposition: attachment; filename="reply.ics"`,
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REPLY",
		"END:VCALENDAR",
		"--b1",
		`Content-Type: text/calendar; method=REQUEST; name="invite.ics"`,
		`Content-Disposition: attachment; filename="invite.ics"`,
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REQUEST",
		"END:VCALENDAR",
	}
	if got := calendarMethodOf(t, declineFrom(dummyReplyThenRequest...)); got != "REQUEST" {
		t.Errorf("a REPLY part beside a REQUEST part stored %q, want REQUEST", got)
	}

	replyBesidePlainEvent := []string{
		"Content-Type: text/calendar; method=REPLY",
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REPLY",
		"END:VCALENDAR",
		"--b1",
		`Content-Type: text/calendar; name="party.ics"`,
		`Content-Disposition: attachment; filename="party.ics"`,
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
	}
	if got := calendarMethodOf(t, declineFrom(replyBesidePlainEvent...)); got != "" {
		t.Errorf("a REPLY part beside a part naming no method stored %q, want none", got)
	}
}

// A part the collector refused was never read, so its header alone sets nothing.
func TestARefusedCalendarPartNamesNoMethod(t *testing.T) {
	t.Parallel()
	var part []string
	for i := range maxParts {
		part = append(part,
			fmt.Sprintf(`Content-Type: application/pdf; name="f%d.pdf"`, i),
			fmt.Sprintf(`Content-Disposition: attachment; filename="f%d.pdf"`, i),
			"",
			"%PDF-1.4",
			"--b1")
	}
	part = append(part,
		`Content-Type: text/calendar; method=REPLY; name="reply.ics"`,
		`Content-Disposition: attachment; filename="reply.ics"`,
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REPLY",
		"END:VCALENDAR")
	if got := calendarMethodOf(t, declineFrom(part...)); got != "" {
		t.Errorf("a calendar part past the file cap stored %q, want none", got)
	}
}
