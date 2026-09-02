// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// A calendar invitation names an EVENT, and the address it is addressed to is
// an ATTENDEE of that event rather than somebody the workspace corresponded
// with.
//
// The distinction has teeth because the two are indistinguishable by direction.
// Google Calendar sends an invitation FROM the organizer — the mailbox owner —
// with the attendee in To, and the provider files a copy in Sent. Nothing in
// that shape says "machine": there is no Auto-Submitted header, no List-* pair,
// no bulk Precedence. So it read as ordinary outbound mail the owner wrote, T1
// took the attendee as an address the workspace writes to, and every person the
// owner had ever invited to anything became a contact — a spouse, a language
// teacher, and the owner's own second address among them.
//
// The evidence is two facts measured on a real mailbox, both present on all 117
// invitations in it: `Sender: Google Calendar <calendar-notification@google.com>`
// while From is the organizer, and a text/calendar part. The iCalendar METHOD is
// deliberately NOT the test — only 83 of those 117 carried `method=REQUEST`, the
// rest being cancellations and updates, so a rule keyed on it would let a third
// of them through.

import (
	"strings"

	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarSenders are the addresses groupware puts in `Sender:` when a person's
// calendar sends on their behalf. The header is the provider speaking, not the
// organizer, and nobody is reachable at any of them.
var calendarSenders = map[string]bool{
	"calendar-notification@google.com": true,
	"calendar-server@google.com":       true,
}

// calendarNotification reports whether this message is groupware speaking for a
// person rather than the person writing.
//
// It reads SENDER, never From. An invitation's From is the organizer, a real
// human with a real address whose ordinary mail must be unaffected; Sender is
// the field RFC 5322 reserves for the agent that actually submitted the
// message, which is exactly the distinction being drawn.
//
// The iCalendar part alone is not enough either: a person can attach an .ics to
// a mail they wrote themselves, and that message IS correspondence. Requiring
// the provider's own Sender address keeps this to messages a machine composed.
func calendarNotification(header mail.Header, hasCalendarPart bool) bool {
	if !calendarSenders[bareAddressOf(header.Get("Sender"))] {
		return false
	}
	// Content-Class is Exchange's spelling of the same fact. Kept because a
	// provider Sender header with no calendar payload is a message ABOUT
	// calendars rather than an invitation.
	if strings.EqualFold(strings.TrimSpace(header.Get("Content-Class")), "urn:content-classes:calendarmessage") {
		return true
	}
	return hasCalendarPart
}

// bareAddressOf reads the address out of a header value that may carry a display
// name: `Google Calendar <calendar-notification@google.com>` is the shape this
// header actually arrives in, and comparing the whole string would match none.
func bareAddressOf(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := mail.ParseAddress(value); err == nil {
		return strings.ToLower(strings.TrimSpace(parsed.Address))
	}
	// A malformed Sender is not a calendar sender. Falling back to the raw
	// string keeps the comparison total without inventing an address.
	return strings.ToLower(value)
}

// recordCounterparty is the human on the other side of this message, or nobody.
//
// Nobody is the honest answer for a calendar invitation. Groupware addressed it
// to an attendee of an event, which is not the same fact as the workspace
// writing to a counterparty — and the ladder reads a counterparty as exactly
// that: evidence of intent toward an address.
//
// The message itself is untouched. It commits, keeps its subject and body, and
// keeps its participants, because being on a meeting is a fact about that
// meeting whether or not it makes anybody a contact. What is withheld is the
// claim that somebody here is a counterparty, which is the claim that was false.
func (m Message) recordCounterparty() connector.Counterparty {
	if m.calendarNotice {
		return connector.Counterparty{}
	}
	return connector.Counterparty{
		Email:           strings.ToLower(strings.TrimSpace(m.counterparty)),
		DisplayName:     m.counterpartyName,
		Domain:          domainOf(m.counterparty),
		Direction:       m.direction,
		ListUnsubscribe: m.listUnsubscribe,
	}.WithOwnerAttestation(m.sentByOwner)
}

// participantExclusion is the address otherParties leaves out because it is
// already one of the two ENDS of the exchange — normally the counterparty.
//
// A calendar invitation has no counterparty, so there is nothing to exclude.
// Excluding the attendee anyway would drop them from the record entirely, and
// they were on the meeting: the calendar connector records the same person as a
// participant from the event side, and mail must not disagree with it.
func participantExclusion(counterparty string, calendarNotice bool) string {
	if calendarNotice {
		return ""
	}
	return counterparty
}
