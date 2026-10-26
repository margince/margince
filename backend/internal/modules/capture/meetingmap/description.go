// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
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

// RerenderStoredBody returns the body capture stores today for a calendar
// meeting body stored before descriptions were rendered. The organizer and
// attendee header stays as stored. A description with HTML tags is rendered
// as descriptionText does, then masked and cut as buildBody does. changed is
// false when the stored description has no tags or renders to itself.
func RerenderStoredBody(stored string) (body string, changed bool) {
	header, desc, found := strings.Cut(stored, "\n\n")
	if !found || !htmlTag.MatchString(desc) {
		return stored, false
	}
	body = header
	if text := descriptionText(desc); text != "" {
		body = header + "\n\n" + text
	}
	body = textcut.BytesMarked(credentialmask.MaskText(body), MaxBodyLen, "…")
	return body, body != stored
}
