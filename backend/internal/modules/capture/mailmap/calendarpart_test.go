// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/pkg/extension"
)

// The invitation itself is a file, and it was being read and thrown away.
//
// A MIME reader is single-pass, so the walk's one look at a part decides
// whether its bytes survive. The unnamed inline route — the shape Google sends —
// read the part to set a flag and returned, which left the flag true and the
// message carrying no calendar file at all. Every other route kept it.
func TestAnUnnamedCalendarPartIsKeptAsAFile(t *testing.T) {
	t.Parallel()
	msg, err := Parse(calendarInviteFixture(), "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !msg.hasCalendarPart {
		t.Fatal("the calendar part was not recognised at all")
	}
	var calendar *Part
	for i := range msg.parts {
		if strings.HasPrefix(msg.parts[i].ContentType, "text/calendar") ||
			strings.HasSuffix(msg.parts[i].Filename, ".ics") {
			calendar = &msg.parts[i]
		}
	}
	if calendar == nil {
		t.Fatalf("the invitation kept no calendar file; parts = %+v", msg.parts)
	}
	if !strings.Contains(string(calendar.Body), "BEGIN:VCALENDAR") {
		t.Errorf("the calendar file is empty of its payload: %q", calendar.Body)
	}
}

// The fact reaches the row the sink writes, not just the parser's own struct.
//
// ToRecord is where a parsed message becomes something the rest of capture can
// read, so a flag that stops at Message is a flag no consumer ever sees.
func TestTheCalendarFactReachesTheRecord(t *testing.T) {
	t.Parallel()
	msg, err := Parse(calendarInviteFixture(), "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("gmail", nil).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("an email record carries ActivityFields")
	}
	if !fields.HasCalendarPart {
		t.Error("the record does not say the message carried a calendar part")
	}
}

// A message with no calendar part says so, or the flag means nothing.
//
// The admit case beside the refuse case: a test that only ever sees invitations
// passes just as well against a parser that hard-codes true.
func TestOrdinaryMailCarriesNoCalendarPart(t *testing.T) {
	t.Parallel()
	plain := crlf(
		"Message-ID: <plain-1@partner.example>",
		"From: Buyer <buyer@partner.example>",
		"To: owner@myco.com",
		"Subject: Re: the proposal",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Looks good, one question.",
		"",
	)
	msg, err := Parse(plain, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if msg.hasCalendarPart {
		t.Error("ordinary mail reported a calendar part")
	}
	if len(msg.parts) != 0 {
		t.Errorf("ordinary mail kept %d files, want none", len(msg.parts))
	}
}

// An unnamed body part is read under the same ceiling a file gets.
//
// Nothing else bounds it: an unnamed inline part never reaches the collector,
// so before this the read was unbounded and one sender chose how much memory a
// capture cost. The assertion is on the KEPT length rather than on a refusal,
// because a truncated body is still the message.
func TestAnOversizedBodyPartIsBounded(t *testing.T) {
	t.Parallel()
	huge := strings.Repeat("A", extension.MaxInboundFileBytes+(1<<20))
	raw := crlf(
		"Message-ID: <huge-1@partner.example>",
		"From: Buyer <buyer@partner.example>",
		"To: owner@myco.com",
		"Subject: a very long note",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		huge,
		"",
	)
	msg, err := Parse(raw, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(msg.body) > extension.MaxInboundFileBytes {
		t.Errorf("body kept %d bytes, above the %d ceiling",
			len(msg.body), extension.MaxInboundFileBytes)
	}
}

// A calendar part does not push a user's file off a full message.
//
// It now takes an ordinal, a count slot and message budget like any other file,
// so a message already at the part cap has one more candidate than it did. The
// files a person sent are what a rep opens; the invitation is evidence. On a
// message at the cap the person's files win.
func TestACalendarPartDoesNotDisplaceAUserFile(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	b.WriteString("Message-ID: <full-1@partner.example>\r\n")
	b.WriteString("From: Buyer <buyer@partner.example>\r\n")
	b.WriteString("To: owner@myco.com\r\n")
	b.WriteString("Subject: everything at once\r\n")
	b.WriteString("Date: Mon, 01 Jun 2026 22:03:51 +0000\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"bb\"\r\n\r\n")
	// The invitation FIRST, which is where the defect lives: a calendar part
	// that takes an early ordinal spends a slot the sender's own files needed,
	// and the last one silently falls off the cap. A fixture with it last
	// passes against the broken parser too.
	b.WriteString("--bb\r\n")
	b.WriteString("Content-Type: text/calendar; charset=UTF-8; method=REQUEST\r\n\r\n")
	b.WriteString("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	for i := 0; i < extension.MaxInboundFiles; i++ {
		b.WriteString("--bb\r\n")
		fmt.Fprintf(&b, "Content-Type: application/pdf; name=\"doc%d.pdf\"\r\n", i)
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=\"doc%d.pdf\"\r\n\r\n", i)
		b.WriteString("PDFBYTES\r\n")
	}
	b.WriteString("--bb--\r\n")

	msg, err := Parse([]byte(b.String()), "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	kept := 0
	for _, p := range msg.parts {
		if strings.HasSuffix(p.Filename, ".pdf") {
			kept++
		}
	}
	if kept != extension.MaxInboundFiles {
		t.Errorf("kept %d of the sender's %d files: the calendar part displaced one",
			kept, extension.MaxInboundFiles)
	}
}
