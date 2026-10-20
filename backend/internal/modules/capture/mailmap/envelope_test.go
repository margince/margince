// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// What the parser hands capture about how a mail was addressed and where its
// links lead. Capture judges Bcc and foreign links from this alone.

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
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
	got, cut := linkHosts("See https://example.com/a", `<a href="https://landing.example.org/b">b</a>`)

	if !slices.Equal(got, []string{"example.com", "landing.example.org"}) || cut {
		t.Errorf("link hosts are %v (cut %v), want the plain part's and the HTML part's", got, cut)
	}
}

// googleRedirect wraps a target in Google's link redirect.
func googleRedirect(target string) string {
	return "https://www.google.com/url?q=" + url.QueryEscape(target) + "&sa=D"
}

func TestLinkTargetFollowsRedirectsToTheirEnd(t *testing.T) {
	deep := "https://landing.example.org/x"
	for range maxRedirectDepth + 1 {
		deep = googleRedirect(deep)
	}
	cases := map[string]struct {
		link     string
		host     string
		resolved bool
	}{
		"a double-wrapped link": {
			link: googleRedirect(googleRedirect("https://landing.example.org/x")), host: "landing.example.org", resolved: true,
		},
		"a userinfo part is no host": {
			link: "https://sender.example.com@landing.example.org/x", host: "landing.example.org", resolved: true,
		},
		"a wrapper with no web target": {link: googleRedirect("nothing"), resolved: false},
		"a chain past the depth limit": {link: deep, resolved: false},
		"a double wrapper ending in no web address": {
			link: googleRedirect(googleRedirect("https://www.google.com/url?q=x")), resolved: false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			host, resolved := linkTarget(tc.link)
			if host != tc.host || resolved != tc.resolved {
				t.Errorf("linkTarget(%s) = %q, %v, want %q, %v", tc.link, host, resolved, tc.host, tc.resolved)
			}
		})
	}
}

// A foreign link after many related ones still reaches the decision. The list
// is marked cut short at its bound.
func TestALongLinkListIsMarkedCutShort(t *testing.T) {
	var body strings.Builder
	for i := range maxLinkHosts {
		fmt.Fprintf(&body, `<a href="https://h%d.example.com/">x</a>`, i)
	}
	body.WriteString(`<a href="https://landing.example.org/">x</a>`)

	hosts, cut := linkHosts("", body.String())

	if len(hosts) != maxLinkHosts || !cut {
		t.Errorf("a list past the bound gave %d hosts, cut %v; want %d hosts and cut", len(hosts), cut, maxLinkHosts)
	}
}

// An HTML body longer than the walk's bound is marked cut short, so a link past
// the bound is not silently lost.
func TestAnHTMLWalkPastItsBoundIsCutShort(t *testing.T) {
	body := strings.Repeat("<b>x</b>", maxLinkTokens) + `<a href="https://landing.example.org/">x</a>`

	if _, cut := linkHosts("", body); !cut {
		t.Error("a walk that stopped at its bound did not say so")
	}
}

// The redirect wrapper and the truncation reach capture through the envelope.
func TestTheEnvelopeCarriesALinkListCutShort(t *testing.T) {
	envelope := envelopeOf(t, "To: "+envelopeOwner, "Content-Type: text/html; charset=utf-8", "",
		`<a href="`+googleRedirect("nothing")+`">x</a>`)

	if !envelope.LinksTruncated {
		t.Error("an unresolved redirect did not mark the envelope's links as cut short")
	}
}

// A <style> or <script> longer than the tokenizer's buffer holds no link. It
// does not cut the walk short, and a link after it is still read. Any other
// token past the buffer still does.
func TestOnlyAnOverlongStyleOrScriptLeavesTheLinkWalkWhole(t *testing.T) {
	filler := strings.Repeat("x", maxTokenLen+1000)
	landing := `<a href="https://landing.example.org/">x</a>`
	cases := map[string]struct {
		body  string
		hosts []string
		cut   bool
	}{
		"a long stylesheet before a link": {
			body: `<STYLE>` + filler + `</Style >` + landing, hosts: []string{"landing.example.org"},
		},
		"a long script before a link": {
			body: `<script type="text/javascript">` + filler + `</script>` + landing, hosts: []string{"landing.example.org"},
		},
		"a long script with no end tag":  {body: landing + `<script>` + filler, hosts: []string{"landing.example.org"}},
		"a long paragraph before a link": {body: `<p>` + filler + `</p>` + landing, cut: true},
		"a long stylesheet, then a long paragraph": {
			body: `<style>` + filler + `</style><p>` + filler + `</p>` + landing, cut: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			hosts, cut := linkHosts("", tc.body)
			if !slices.Equal(hosts, tc.hosts) || cut != tc.cut {
				t.Errorf("link hosts are %v (cut %v), want %v (cut %v)", hosts, cut, tc.hosts, tc.cut)
			}
		})
	}
}

// The buffer can run out inside the end tag itself. The walk still finds that
// end tag and the link after it.
func TestAStylesheetWhoseEndTagStraddlesTheBufferLeavesTheWalkWhole(t *testing.T) {
	for short := 1; short <= len("</style>")+1; short++ {
		body := `<style>` + strings.Repeat("x", maxTokenLen-short) + `</style><a href="https://landing.example.org/">x</a>`

		if hosts, cut := linkHosts("", body); !slices.Equal(hosts, []string{"landing.example.org"}) || cut {
			t.Errorf("content %d bytes short of the buffer: link hosts %v (cut %v), want the link", short, hosts, cut)
		}
	}
}

// A "</stylex>" does not end a stylesheet, so the <script> after it is still
// stylesheet text and cannot hide the link that follows the real end tag.
func TestALookalikeEndTagDoesNotEndAStylesheet(t *testing.T) {
	body := `<style>` + strings.Repeat("x", maxTokenLen+1000) + `</stylex><script></style>` +
		`<a href="https://landing.example.org/">x</a></script>`

	if hosts, cut := linkHosts("", body); !slices.Equal(hosts, []string{"landing.example.org"}) || cut {
		t.Errorf("link hosts are %v (cut %v), want the link after the real end tag", hosts, cut)
	}
}

// A mail client renders with scripting off, so a link inside <noscript> is
// one the reader sees.
func TestALinkInsideNoscriptIsRead(t *testing.T) {
	hosts, cut := linkHosts("", `<noscript><a href="https://landing.example.org/">Open</a></noscript>`)

	if !slices.Equal(hosts, []string{"landing.example.org"}) || cut {
		t.Errorf("link hosts are %v (cut %v), want the link inside noscript", hosts, cut)
	}
}

// A To or Cc header that will not parse may have named the reader. The
// envelope says so, and does not read as a Bcc copy.
func TestAnUnreadableRecipientHeaderIsMarked(t *testing.T) {
	cases := map[string]struct {
		headers []string
		want    bool
	}{
		"a To that will not parse": {headers: []string{"To: Reader <" + envelopeOwner}, want: true},
		"a Cc that will not parse": {
			headers: []string{"To: other@example.com", "Cc: \"Reader <" + envelopeOwner + ">"}, want: true,
		},
		"headers that parse": {headers: []string{"To: other@example.com", "Cc: " + envelopeOwner}},
		"no Cc at all":       {headers: []string{"To: other@example.com"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			envelope := envelopeOf(t, append(tc.headers, plainBody("Hello")...)...)
			if envelope.RecipientsUnreadable != tc.want {
				t.Errorf("recipients unreadable = %v, want %v", envelope.RecipientsUnreadable, tc.want)
			}
		})
	}
}
