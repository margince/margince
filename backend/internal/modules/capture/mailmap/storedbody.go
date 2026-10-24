// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// recordBody is the activity body capture stores for this message with text:
// the From/To lines Margince adds, then the text, cut to limit. cut reports
// that text was left out.
func (m Message) recordBody(text string, limit int) (body string, cut bool) {
	body = fmt.Sprintf("From: %s\nTo: %s", orDash(m.from), orDash(m.to))
	if text != "" {
		body += "\n\n" + text
	}
	return textcut.BytesMarked(body, limit, "…"), len(body) > limit
}

// storedBody is the body capture stores and how much of the text it holds. That
// is the parse's verdict, made partial when the cut at maxBodyLen left text out.
// Fresh capture and the legacy assessment both read it.
func (m Message) storedBody() (string, capture.BodyCompleteness) {
	body, cut := m.recordBody(m.body, maxBodyLen)
	return body, cutCompleteness(m.bodyCompleteness, cut)
}

func cutCompleteness(parsed capture.BodyCompleteness, cut bool) capture.BodyCompleteness {
	if cut && parsed == capture.BodyComplete {
		return capture.BodyPartial
	}
	return parsed
}

// AssessStoredBody says how much of a mail's text a body stored from raw
// holds, by the rule capture applies today.
//   - Equal to what capture stores now: that body's completeness.
//   - Equal to the same text cut at legacyMaxBodyLen: partial when the cut
//     left text out.
//   - Only the From/To lines of a mail with text: unreadable, because an older
//     parser lost that text.
//
// ok is false for any other stored body. An edit or an older parser wrote it,
// and nothing here can vouch for it.
func AssessStoredBody(raw []byte, stored string) (capture.BodyCompleteness, bool) {
	msg, err := Parse(raw, "")
	if err != nil {
		return "", false
	}
	if current, completeness := msg.storedBody(); stored == current {
		return completeness, true
	}
	if legacy, cut := msg.recordBody(msg.body, legacyMaxBodyLen); stored == legacy {
		return cutCompleteness(msg.bodyCompleteness, cut), true
	}
	if headerOnly, _ := msg.recordBody("", maxBodyLen); stored == headerOnly && msg.body != "" {
		return capture.BodyUnreadable, true
	}
	return "", false
}

// RecoverStoredBody re-derives the body capture stores now from raw. It is for a
// stored body an older parser or the older cap left short. That is only the
// From/To lines, or the text cut at legacyMaxBodyLen. ok is false for any other
// stored body, and for an original that holds no more text than is stored.
func RecoverStoredBody(raw []byte, stored string) (body string, completeness capture.BodyCompleteness, ok bool) {
	msg, err := Parse(raw, "")
	if err != nil || msg.body == "" {
		return "", "", false
	}
	headerOnly, _ := msg.recordBody("", maxBodyLen)
	legacy, cut := msg.recordBody(msg.body, legacyMaxBodyLen)
	if stored != headerOnly && (stored != legacy || !cut) {
		return "", "", false
	}
	body, completeness = msg.storedBody()
	return body, completeness, body != stored
}
