// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// The address a message was DELIVERED to, when the receiving infrastructure
// said so and a sender could not have.
//
// A forwarding alias is invisible everywhere else. It is never the From of
// anything the mailbox sends, so the send-side discovery that finds a send-as
// alias cannot reach it, and a seat whose mail arrives through a previous
// employer's address or a personal domain keeps appearing in their own CRM as
// a contact.

import (
	"strings"

	"github.com/emersion/go-message/mail"
)

// TopDeliveredTo answers the address the receiving server recorded this message
// as delivered to, and the empty string when no such claim can be trusted.
//
// POSITION IS THE WHOLE DEFENCE, and it is why this is one predicate rather
// than a header read at each call site.
//
// A receiving MTA PREPENDS. Whatever a sender submits — including a
// `Delivered-To` naming somebody else — every receiving hop's `Received` is
// written above it. So a `Delivered-To` that sits above the first `Received`
// was written by the infrastructure that actually delivered the message, and
// one that sits below it is in a position the sender could have authored.
//
// Getting that backwards is not a missed alias; it lets a sender declare
// themselves to be the mailbox owner, which would suppress their own capture
// and shift what the owner's self-set covers. So the refusal is the default:
// anything this function cannot place above the first `Received` answers
// empty, and a caller has nothing to act on.
//
// Only the FIRST occurrence is ever read. A message carrying several is a
// message that crossed several hops, and the topmost is the last one written —
// the final delivery, which is the one that says this address reaches this
// mailbox.
func TopDeliveredTo(header mail.Header) string {
	fields := header.Fields()
	for fields.Next() {
		switch strings.ToLower(fields.Key()) {
		case "delivered-to":
			return strings.TrimSpace(fields.Value())
		case "received":
			// A hop was recorded before any delivery claim. Everything from
			// here down is territory the sender submitted, so there is no
			// trustworthy claim in this message.
			return ""
		}
	}
	return ""
}
