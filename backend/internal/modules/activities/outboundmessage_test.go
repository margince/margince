// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"
)

// The engine reads what the sender wrote for an offer: the typed subject and
// body, not the rendered ones, and the names of the files they attached.
func TestTheEngineIsAskedAboutTheAuthoredTextAndTheFileNames(t *testing.T) {
	m := outboundMessage{
		in:    SendEmailInput{Recipients: []string{"dana@buyer.test"}, Subject: "Re: Termin", Body: "Siehe Anhang."},
		body:  "Siehe Anhang.\n\n-- \nAn Tran\nUnsubscribe: https://example.test/u",
		files: []OutboundFile{{Filename: "Angebot_2026.pdf"}, {Filename: "Agenda.pdf"}},
	}
	req := m.authorization(SendOrigin{})
	if req.AuthoredSubject != "Re: Termin" || req.AuthoredBody != "Siehe Anhang." {
		t.Errorf("authored text = %q / %q, want what the sender typed", req.AuthoredSubject, req.AuthoredBody)
	}
	if !slices.Equal(req.AuthoredFileNames, []string{"Angebot_2026.pdf", "Agenda.pdf"}) {
		t.Errorf("file names = %v, want both attachments", req.AuthoredFileNames)
	}
}
