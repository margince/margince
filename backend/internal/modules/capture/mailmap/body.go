// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Walking a message's parts: what the body says, what files it carried, and
// whether a calendar payload was among them.

import (
	"errors"
	"io"
	"strings"

	"github.com/emersion/go-message/mail"
)

// extractText returns the message's plain-text body and the files it carried.
//
// It prefers a text/plain part, rendering text/html to text only when no plain
// part exists, so an HTML-only newsletter still yields readable text. Attachment parts are collected on the SAME walk: a MIME reader
// is single-pass, so a second walk would mean holding or re-parsing the whole
// message to find files this one already stepped over.
func extractText(reader *mail.Reader) (string, []Part, []PartDrop, bool) {
	var plain, html string
	calendar := false
	files := newCollector()
	for {
		if files.exhausted() {
			// Past any real message. Everything beyond is unread and reported
			// as such rather than walked, because walking it is the work an
			// unauthenticated sender was trying to buy.
			files.truncated()
			break
		}
		part, err := reader.NextPart()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				// A structural failure ends the walk, so any file after this
				// point is lost. Recorded rather than passed over: a message
				// whose files went missing must not read like one that carried
				// none (DOC-AC-12).
				files.truncated()
			}
			break
		}
		if attached, ok := part.Header.(*mail.AttachmentHeader); ok {
			calendar = calendar || declaresCalendar(attached)
			files.take(attached, part.Body)
			continue
		}
		inline, ok := part.Header.(*mail.InlineHeader)
		if !ok {
			continue
		}
		if readInlinePart(inline, part.Body, files, &plain, &html, &calendar) {
			continue
		}
	}
	return bodyText(plain, html), files.parts, files.drops(), calendar
}

func bodyText(plain, html string) string {
	if strings.TrimSpace(plain) != "" {
		return strings.TrimSpace(plain)
	}
	if html != "" {
		return htmlToText(html)
	}
	return ""
}

// A calendar payload arrives by three routes: an unnamed inline part (the shape
// Google sends), a named inline part, and a declared attachment. All three are
// the same evidence — an invitation — so all three are read, and the two named
// routes are read before the collector consumes the part.
func isCalendarType(contentType string) bool {
	return strings.HasPrefix(strings.ToLower(contentType), "text/calendar")
}

func declaresCalendar(header *mail.AttachmentHeader) bool {
	declared, _, err := header.ContentType()
	if err != nil {
		// A malformed Content-Type is no claim at all. Whether this message is
		// an invitation is then decided by its other parts and its headers.
		return false
	}
	return isCalendarType(declared)
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
	plain, html *string, calendar *bool,
) bool {
	contentType, _, err := inline.ContentType()
	if err != nil {
		return false
	}
	// An INLINE part with a filename is a file. Several mail clients send PDFs
	// and images that way as a matter of course, and reading only
	// Content-Disposition: attachment loses them with nothing recorded — the
	// sender chose how to render it, not whether we keep it.
	if name := inlineFilename(inline); name != "" {
		*calendar = *calendar || isCalendarType(contentType)
		files.takeInline(inline, name, body)
		return true
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return false
	}
	switch {
	case strings.HasPrefix(contentType, "text/plain") && *plain == "":
		*plain = string(content)
	case strings.HasPrefix(contentType, "text/html") && *html == "":
		*html = string(content)
	case isCalendarType(contentType):
		*calendar = true
	}
	return true
}
