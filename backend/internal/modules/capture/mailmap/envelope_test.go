// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// What the parser hands capture about how a mail was addressed and where its
// links lead. Capture judges Bcc and foreign links from this alone.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

const envelopeOwner = "reader@ourco.example"

// envelopeOf parses one inbound mail and returns its envelope. The lines are
// the headers that vary, then the body part's own header, a blank and its text.
func envelopeOf(t *testing.T, rest ...string) *capture.MailEnvelope {
	t.Helper()
	lines := append([]string{
		"From: Sam Sender <sender@example.com>",
		"Subject: View the document",
		"Date: Wed, 04 Jun 2026 09:00:00 +0000",
		"Message-ID: <env-1@example.com>",
	}, rest...)
	lines = append(lines, "")
	msg, err := Parse(crlf(lines...), envelopeOwner)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("imap", nil).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("a mail did not map to activity fields")
	}
	return fields.Envelope
}

func plainBody(text string) []string {
	return []string{"Content-Type: text/plain; charset=utf-8", "", text}
}

func TestABccCopyNamesOnlyOtherRecipients(t *testing.T) {
	envelope := envelopeOf(t, append([]string{"To: sender@example.com"}, plainBody("Hello")...)...)

	if envelope == nil {
		t.Fatal("an ordinary inbound mail carried no envelope")
	}
	if slices.Contains(envelope.OpenRecipients, envelopeOwner) {
		t.Errorf("open recipients %v name the owner, who got this copy only by Bcc", envelope.OpenRecipients)
	}
	if !slices.Contains(envelope.OpenRecipients, "sender@example.com") {
		t.Errorf("open recipients %v lost the To address", envelope.OpenRecipients)
	}
}

func TestAnUndisclosedRecipientListHasNoOpenRecipients(t *testing.T) {
	envelope := envelopeOf(t, append([]string{"To: undisclosed-recipients:;"}, plainBody("Hello")...)...)

	if envelope == nil {
		t.Fatal("a mail to undisclosed recipients carried no envelope")
	}
	if len(envelope.OpenRecipients) != 0 {
		t.Errorf("an undisclosed list gave open recipients %v, want none", envelope.OpenRecipients)
	}
}

func TestAReaderInCcIsAnOpenRecipient(t *testing.T) {
	envelope := envelopeOf(t, append([]string{"To: other@example.com", "Cc: Reader <" + envelopeOwner + ">"},
		plainBody("Hello")...)...)

	if !slices.Contains(envelope.OpenRecipients, envelopeOwner) {
		t.Errorf("open recipients %v leave out the owner named in Cc", envelope.OpenRecipients)
	}
}

// The owner's absence from a list's To says nothing about a Bcc, so list mail
// carries no envelope at all.
func TestListMailCarriesNoEnvelope(t *testing.T) {
	envelope := envelopeOf(t, append([]string{"To: list@example.com", "List-Id: <list.example.com>"},
		plainBody("Hello")...)...)

	if envelope != nil {
		t.Errorf("list mail carried an envelope %+v, want none", envelope)
	}
}

func TestLinkHostsReadTheRealTargetOfEachLink(t *testing.T) {
	cases := map[string]struct {
		body []string
		want []string
	}{
		"a redirect wrapper is unwrapped": {
			body: []string{
				"Content-Type: text/html; charset=utf-8", "",
				`<p><a href="https://www.google.com/url?q=https://landing.example.org/doc/index.html&amp;sa=D&amp;source=calendar">View RFP Document</a></p>`,
			},
			want: []string{"landing.example.org"},
		},
		"a link to the sender's own domain": {
			body: []string{"Content-Type: text/html; charset=utf-8", "", `<a href="https://example.com/offer">Offer</a>`},
			want: []string{"example.com"},
		},
		"a link to a subdomain of the sender": {
			body: []string{"Content-Type: text/html; charset=utf-8", "", `<a href="https://docs.example.com/a">Docs</a>`},
			want: []string{"docs.example.com"},
		},
		"a link written out in a plain-text body": {
			body: plainBody("Please open https://landing.example.org/doc today."),
			want: []string{"landing.example.org"},
		},
		"a mailto link is no web link": {
			body: []string{"Content-Type: text/html; charset=utf-8", "", `<a href="mailto:sender@example.com">Mail</a>`},
			want: nil,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			envelope := envelopeOf(t, append([]string{"To: " + envelopeOwner}, tc.body...)...)

			if !slices.Equal(envelope.LinkHosts, tc.want) {
				t.Errorf("link hosts are %v, want %v", envelope.LinkHosts, tc.want)
			}
		})
	}
}

// A sender can put one link in the plain part and another in the HTML part a
// mail client shows. Both are read.
func TestLinkHostsReadBothBodies(t *testing.T) {
	got := linkHosts("See https://example.com/a", `<a href="https://landing.example.org/b">b</a>`)

	if !slices.Equal(got, []string{"example.com", "landing.example.org"}) {
		t.Errorf("link hosts are %v, want the plain part's and the HTML part's", got)
	}
}
