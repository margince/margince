// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"
)

func TestAWithheldLinkStaysInTheDeliveryAndLeavesTheRecordedCopy(t *testing.T) {
	const link = "https://crm.example/#/book/manage-token-for-the-host"
	text := "Reschedule or cancel\n" + link
	p := PreparedSend{message: outboundMessage{body: text, recordedBody: text}}
	got := p.withholdingFromRecord(link)
	if got.message.body != text {
		t.Errorf("delivery body = %q, want the live link kept", got.message.body)
	}
	if want := "Reschedule or cancel\n" + redactedMeetingLink; got.message.recordedBody != want {
		t.Errorf("recorded body = %q, want %q", got.message.recordedBody, want)
	}
	if kept := p.withholdingFromRecord("").message.recordedBody; kept != text {
		t.Errorf("an empty secret rewrote the record to %q", kept)
	}
}

// The engine reads what the sender wrote for an offer. That is the typed
// subject and body, not the rendered ones, and the attached files' names.
func TestTheEngineIsAskedAboutTheAuthoredTextAndTheFileNames(t *testing.T) {
	m := outboundMessage{
		in:    SendEmailInput{Recipients: []string{"dana@buyer.test"}, Subject: "Re: Termin", Body: "Siehe Anhang.", HTMLBody: "<p>Siehe Anhang.</p>"},
		body:  "Siehe Anhang.\n\n-- \nAn Tran\nUnsubscribe: https://example.test/u",
		files: []OutboundFile{{Filename: "Angebot_2026.pdf"}, {Filename: "Agenda.pdf"}},
	}
	req := m.authorization(SendOrigin{})
	if req.AuthoredSubject != "Re: Termin" || req.AuthoredBody != "Siehe Anhang." || req.AuthoredHTML != "<p>Siehe Anhang.</p>" {
		t.Errorf("authored text = %q / %q, want what the sender typed", req.AuthoredSubject, req.AuthoredBody)
	}
	if !slices.Equal(req.AuthoredFileNames, []string{"Angebot_2026.pdf", "Agenda.pdf"}) {
		t.Errorf("file names = %v, want both attachments", req.AuthoredFileNames)
	}
}
