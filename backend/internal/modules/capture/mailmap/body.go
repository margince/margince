// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Walking a message's parts: what the body says, what files it carried, and
// whether a calendar payload was among them.

import (
	"io"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// extractText returns the message's plain-text body, its link hosts and the
// files it carried.
//
// It prefers a text/plain part, rendering text/html to text only when no plain
// part exists, so an HTML-only newsletter still yields readable text. Attachment parts are collected on the SAME walk: a MIME reader
// is single-pass, so a second walk would mean holding or re-parsing the whole
// message to find files this one already stepped over.
func extractText(root *message.Entity) walkedBody {
	walk := &partWalker{files: newCollector()}
	walk.run(root)
	// Everything the sender named has had its claim on the bounds; what was
	// held back takes what is left.
	walk.files.settle()
	text, completeness := walk.text(), walk.completeness()
	links, linksCut := walkLinks(walk.plain, walk.html)
	if text == "" {
		// A forward as attachment says nothing of its own; the original does.
		// Its words are the original's, not the sender's, and the completeness
		// says so: automatic work never reads them as this sender's.
		inner := walk.readForwarded()
		switch {
		case inner == nil:
		case inner.text() != "":
			text, completeness = inner.text(), capture.BodyForwarded
			links, linksCut = walkLinks(inner.plain, inner.html)
		case inner.completeness() == capture.BodyUnreadable:
			completeness = capture.BodyUnreadable
		}
	}
	return walkedBody{
		text: text, links: links, linksCut: linksCut,
		parts: walk.files.parts, drops: walk.files.drops(), calendar: walk.calendar,
		completeness: completeness,
	}
}

// walkedBody is what one walk over a message's parts found: the text, the
// hosts its links lead to, its files and its calendar payload.
type walkedBody struct {
	text         string
	links        []capture.MailLink
	linksCut     bool
	parts        []Part
	drops        []PartDrop
	calendar     calendarPart
	completeness capture.BodyCompleteness
}

// bodyText is the text a mail is read by: its plain part, or its HTML rendered
// when the plain part is blank. cut reports a render that stopped early.
func bodyText(plain, html string) (text string, cut bool) {
	if strings.TrimSpace(plain) != "" {
		return strings.TrimSpace(plain), false
	}
	if html != "" {
		return renderHTML(html)
	}
	return "", false
}

// A calendar payload arrives by three routes: an unnamed inline part (the shape
// Google sends), a named inline part, and a declared attachment. All three are
// the same evidence — an invitation — so all three set the flag AND hand their
// bytes to the collector. The unnamed route did neither for the file until the
// order in readInlinePart put the collector ahead of the body read.
func isCalendarType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "text/calendar")
}

// contentTyped is a part header, attachment or inline alike.
type contentTyped interface {
	ContentType() (string, map[string]string, error)
}

// declaresCalendar reports whether a part declares a calendar payload, with the
// Content-Type parameters that carry its iCalendar method.
func declaresCalendar(header contentTyped) (map[string]string, bool) {
	declared, params, err := header.ContentType()
	if err != nil {
		// A malformed Content-Type is no claim at all. Whether this message is
		// an invitation is then decided by its other parts and its headers.
		return nil, false
	}
	return params, isCalendarType(declared)
}

// readInlinePart takes one inline part: a named one is a file, an unnamed one is
// body text or a calendar payload. It reports whether the part was consumed at
// all, which is false only for a part whose type could not be read.
//
// The body's two halves and the calendar flag are pointers because a MIME reader
// is single-pass: this is the only look any part gets, and what the walk learns
// has to leave with it.
func readInlinePart(
	inline *mail.InlineHeader, body io.Reader, files *collector,
	plain, html *string, calendar *calendarPart,
) bool {
	contentType, params, err := inline.ContentType()
	if err != nil {
		return false
	}
	// An INLINE part with a filename is a file. Several mail clients send PDFs
	// and images that way as a matter of course, and reading only
	// Content-Disposition: attachment loses them with nothing recorded — the
	// sender chose how to render it, not whether we keep it.
	if name := inlineFilename(inline); name != "" {
		files.takeInline(inline, name, body)
		if isCalendarType(contentType) {
			calendar.see(params, files.justAdmitted())
		}
		return true
	}
	// An UNNAMED calendar part is the invitation itself. It is read here — the
	// reader is single-pass, so this is its only look — and admitted to the
	// collector only once the walk is over.
	//
	// LAST, deliberately. A part admitted mid-walk spends an ordinal, a count
	// slot and message budget that the sender's own files needed, so an
	// invitation arriving before twenty attachments silently pushed the
	// twentieth off the cap. The files a contact chose to send are what a rep
	// opens; the invitation is evidence, and evidence yields.
	if isCalendarType(contentType) {
		content := readBoundedRaw(body)
		calendar.see(params, content)
		files.holdCalendar(content)
		return true
	}
	content, err := readBounded(body)
	if err != nil {
		return false
	}
	switch {
	case strings.HasPrefix(contentType, "text/plain") && *plain == "":
		*plain = string(toUTF8(content, params["charset"]))
	case strings.HasPrefix(contentType, "text/html") && *html == "":
		*html = string(toUTF8(content, params["charset"]))
	}
	return true
}

// unnamedCalendarFilename names the invitation a sender rendered inline without
// naming. Every client that sends this shape calls it the same thing.
const unnamedCalendarFilename = "invite.ics"

// calendarContentType is what the held part DECLARED. The collector sniffs the
// bytes and keeps this only where the two disagree, exactly as it does for a
// part the sender named.
const calendarContentType = "text/calendar"

// readBoundedRaw reads a part under the per-part ceiling and reports nothing:
// an unreadable calendar payload is an absent one, which the collector's own
// bounds already account for.
func readBoundedRaw(body io.Reader) []byte {
	content, err := readBounded(body)
	if err != nil {
		return nil
	}
	return content
}

// readBounded reads one body part under the same per-part ceiling a file gets.
//
// An unnamed inline part is body text and never reaches the collector, so
// nothing else bounds it: an unbounded read here lets one sender choose how
// much memory a capture costs. Truncating rather than refusing keeps the body
// a message actually carried — a 25 MiB text part is a hostile message, and the
// first 25 MiB of it is more than enough to read.
func readBounded(body io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(body, maxPartBytes))
}
