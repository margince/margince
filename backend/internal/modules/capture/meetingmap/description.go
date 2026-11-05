// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture/htmltext"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// htmlTag matches a real element (a known name, opening or closing) or a
// comment. A bracketed address or link such as <a@b.c> or <https://x.test> is
// plain text, and rendering it would drop the bracketed value.
var htmlTag = regexp.MustCompile(`(?is)<!--.*?-->|</?(?:` + htmlElements + `)(?:\s[^<>]*)?/?>`)

// cutOpeningTag matches a known opening tag that an older stored-body cut
// left unfinished at the very end, marked by the cut's ellipsis.
var cutOpeningTag = regexp.MustCompile(`(?is)<(?:` + htmlElements + `)(?:\s[^<>]*)?…$`)

const htmlElements = `a|abbr|b|blockquote|body|br|button|center|code|div|em|font|h[1-6]|head|hr|html|i|img|li|ol|p|pre|s|small|span|strike|strong|style|script|sub|sup|table|tbody|td|th|thead|tr|u|ul`

// descriptionText is the text a reader sees for a calendar description. Google
// returns HTML for a description made by a booking page, so a description with
// tags goes through the renderer mail capture uses.
func descriptionText(desc string) string {
	if htmlTag.MatchString(desc) {
		desc, _ = htmltext.Render(desc)
	}
	return strings.TrimSpace(desc)
}

// agendaFingerprint identifies the description as the provider sent it, before
// rendering and masking, so a change to either never reads as a new agenda.
// Space around the text carries no agenda, so it never asks anyone again. An
// empty description has a fingerprint too, because an empty slot keeps the stored one.
func agendaFingerprint(desc string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(desc)))
	return hex.EncodeToString(sum[:16])
}

// RerenderStoredBody returns the body capture stores today for a calendar
// meeting body stored before descriptions were rendered. The organizer and
// attendee header stays as stored. A description with HTML tags is rendered
// as descriptionText does, then masked and cut as buildBody does. changed is
// false when the stored description has no tags or renders to itself.
func RerenderStoredBody(stored string) (body string, changed bool) {
	header, desc, found := strings.Cut(stored, "\n\n")
	if !found {
		return stored, false
	}
	var text string
	switch {
	case htmlTag.MatchString(desc):
		text = descriptionText(desc)
	case cutOpeningTag.MatchString(desc):
		// The cut fell inside the first tag, so no tag is whole. The renderer
		// still reads the text before it.
		rendered, _ := htmltext.Render(desc)
		text = strings.TrimSpace(rendered)
	default:
		return stored, false
	}
	body = header
	if text != "" {
		body = header + "\n\n" + text
	}
	body = textcut.BytesMarked(credentialmask.MaskText(body), MaxBodyLen, "…")
	return body, body != stored
}
