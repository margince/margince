// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "strings"

// A captured email is keyed (EmailSourceSystem, Message-ID), and that header is
// typed by whoever sent the mail. Two mailboxes genuinely holding one message
// and a row filed by somebody who only knew what to type collide identically,
// so a seat that cannot prove anything about the incumbent files the message
// under a key scoped to itself instead of losing it.
//
// The composition and its inverse live HERE, in the port both sides import,
// because they are one rule spelled on two sides of a boundary: capture writes
// the key and activities reads a stored one back as the In-Reply-To of a reply.
// A second spelling in either module is how a reply comes to thread onto
// nothing.

// mailSeatMarker separates the shared Message-ID from the seat that could not
// claim it. A space, because ValidMessageID refuses one: no genuine Message-ID
// can carry this sequence, so a scoped key never collides with a bare key
// somebody files later, and a scoped key never passes as an identity.
const mailSeatMarker = " seat "

// SeatScopedMailKey is the natural key one seat files a message under when the
// shared key is held by a row they can prove nothing about. Deterministic, so
// the same seat's next sync of the same message is the ordinary replay.
func SeatScopedMailKey(messageID, seat string) string {
	return messageID + mailSeatMarker + seat
}

// SharedMailKey is the Message-ID behind a stored natural key, whether the row
// holds the shared key or one seat's scoped copy of it.
//
// Every reader that thinks in Message-IDs goes through this: the seat-scoping
// is an artefact of who filed the row, never of which message it is.
func SharedMailKey(sourceID string) string {
	if at := strings.Index(sourceID, mailSeatMarker); at >= 0 {
		return sourceID[:at]
	}
	return sourceID
}
