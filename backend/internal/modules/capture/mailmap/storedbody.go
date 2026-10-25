// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// recordBody is the activity body capture stores for this message with text:
// the From/To lines Margince adds, then the text, credentials masked, cut to
// limit. cut reports that text was left out. The mask runs before the cut, so a
// key the cut would split is still found whole.
func (m Message) recordBody(text string, limit int) (body string, cut bool) {
	return cutBody(credentialmask.MaskText(m.unmaskedBody(text)), limit)
}

func (m Message) unmaskedBody(text string) string {
	body := fmt.Sprintf("From: %s\nTo: %s", orDash(m.from), orDash(m.to))
	if text != "" {
		body += "\n\n" + text
	}
	return body
}

// masksCredentials says the stored subject or body differs from this message's
// own text because credentials were masked.
func (m Message) masksCredentials() bool {
	full := m.unmaskedBody(m.body)
	return credentialmask.MaskText(m.subject) != m.subject || credentialmask.MaskText(full) != full
}

func cutBody(body string, limit int) (string, bool) {
	return textcut.BytesMarked(body, limit, "…"), len(body) > limit
}

// storedShape is one body capture has stored for a text at a limit.
type storedShape struct {
	body string
	cut  bool
}

// storedShapes lists the bodies capture has stored for text at limit. That is
// today's body, the body stored before credentials were masked, and that older
// body as a later repair masked it in place.
func (m Message) storedShapes(text string, limit int) []storedShape {
	body, cut := m.recordBody(text, limit)
	unmasked, unmaskedCut := cutBody(m.unmaskedBody(text), limit)
	return []storedShape{{body, cut}, {unmasked, unmaskedCut}, {credentialmask.MaskText(unmasked), unmaskedCut}}
}

// heldShape is the shape among shapes that stored is, if any.
func heldShape(shapes []storedShape, stored string) (storedShape, bool) {
	for _, shape := range shapes {
		if shape.body == stored {
			return shape, true
		}
	}
	return storedShape{}, false
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
// Each counts in every shape storedShapes lists. ok is false for any other
// stored body. An edit or an older parser wrote it, and nothing here can vouch
// for it.
func AssessStoredBody(raw []byte, stored string) (capture.BodyCompleteness, bool) {
	msg, err := Parse(raw, "")
	if err != nil {
		return "", false
	}
	for _, limit := range []int{maxBodyLen, legacyMaxBodyLen} {
		if shape, ok := heldShape(msg.storedShapes(msg.body, limit), stored); ok {
			return cutCompleteness(msg.bodyCompleteness, shape.cut), true
		}
	}
	if _, ok := heldShape(msg.storedShapes("", maxBodyLen), stored); ok && msg.body != "" {
		return capture.BodyUnreadable, true
	}
	return "", false
}

// RecoverStoredBody re-derives the body capture stores now from raw. It is for a
// stored body an older parser or the older cap left short. That is only the
// From/To lines, or the text cut at legacyMaxBodyLen, in any shape storedShapes
// lists. ok is false for any other stored body, and for an original that holds no more text than is stored.
func RecoverStoredBody(raw []byte, stored string) (body string, completeness capture.BodyCompleteness, ok bool) {
	msg, err := Parse(raw, "")
	if err != nil || msg.body == "" {
		return "", "", false
	}
	_, headerOnly := heldShape(msg.storedShapes("", maxBodyLen), stored)
	legacy, isLegacy := heldShape(msg.storedShapes(msg.body, legacyMaxBodyLen), stored)
	if !headerOnly && (!isLegacy || !legacy.cut) {
		return "", "", false
	}
	body, completeness = msg.storedBody()
	return body, completeness, body != stored
}
