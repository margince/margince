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
// took the attendee as an address the workspace writes to, and every contact the
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
	"cmp"
	"strings"

	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarmethod"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarSenders are the addresses groupware puts in `Sender:` when a contact's
// calendar sends on their behalf. The header is the provider speaking, not the
// organizer, and nobody is reachable at any of them.
var calendarSenders = map[string]bool{
	// Google, measured. Both spellings ship: the first on invitations, the
	// second on some notification classes.
	"calendar-notification@google.com": true,
	"calendar-server@google.com":       true,
	// Apple iCloud calendar invitations.
	"calendar-notification@icloud.com": true,
	"noreply@calendar.icloud.com":      true,
}

// exchangeCalendarClass is how Exchange and Outlook say the same thing. They
// name no calendar Sender address at all — the organizer's own server sends —
// so this header is the only evidence, and it is checked INSTEAD of the sender
// allowlist rather than after it.
const exchangeCalendarClass = "urn:content-classes:calendarmessage"

// calendarNotification reports whether this message is groupware speaking for a
// contact rather than the contact writing.
//
// It reads SENDER, never From. An invitation's From is the organizer, a real
// human with a real address whose ordinary mail must be unaffected; Sender is
// the field RFC 5322 reserves for the agent that actually submitted the
// message, which is exactly the distinction being drawn.
//
// The iCalendar part alone is not enough either: a contact can attach an .ics to
// a mail they wrote themselves, and that message IS correspondence. Requiring
// the provider's own Sender address keeps this to messages a machine composed.
func calendarNotification(header mail.Header, hasCalendarPart bool) bool {
	// Exchange and Outlook say it outright, and name no calendar Sender at all.
	// Checked first, and independently of the allowlist: gating it behind a
	// Google address made this arm unreachable for the only senders that write
	// it.
	if strings.EqualFold(strings.TrimSpace(header.Get("Content-Class")), exchangeCalendarClass) {
		return true
	}
	if !calendarSenders[bareAddressOf(header.Get("Sender"))] {
		return false
	}
	// A provider Sender header with no calendar payload is a message ABOUT
	// calendars — a digest, a reminder digest — rather than an invitation.
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
// they were on the meeting: the calendar connector records the same contact as a
// participant from the event side, and mail must not disagree with it.
func participantExclusion(counterparty string, calendarNotice bool) string {
	if calendarNotice {
		return ""
	}
	return counterparty
}

// calendarPart is what the MIME walk learns about a message's calendar parts.
type calendarPart struct {
	present bool
	parts   []partMethods
}

// partMethods is what one calendar part declares: the known methods its
// Content-Type and its body name, and whether the two disagree.
type partMethods struct {
	methods  []string
	disputed bool
}

// see records one calendar part. A part whose bytes the collector refused names
// no method: its header alone is a claim nothing else can check.
func (c *calendarPart) see(params map[string]string, body []byte) {
	c.present = true
	if body == nil {
		return
	}
	c.parts = append(c.parts, declaredMethods(params, body))
}

// method is the message's calendar method, resolved against the sender. It is
// a response only when every part names one and no part contradicts itself;
// otherwise the first non-response method wins, or none.
func (c calendarPart) method() string {
	response := len(c.parts) > 0
	firstResponse, firstOther := "", ""
	for _, part := range c.parts {
		if part.disputed || len(part.methods) == 0 {
			response = false
		}
		for _, method := range part.methods {
			switch {
			case !calendarmethod.IsResponse(method):
				response = false
				firstOther = cmp.Or(firstOther, method)
			case firstResponse == "":
				firstResponse = method
			}
		}
	}
	if response {
		return firstResponse
	}
	return firstOther
}

// declaredMethods reads one part's Content-Type method parameter and the
// unfolded METHOD line inside its BEGIN:VCALENDAR. RFC 6047 requires the two to
// match, so a mismatch, or a value the RFC does not define, disputes the part.
func declaredMethods(params map[string]string, body []byte) partMethods {
	var out partMethods
	declare := func(raw string) {
		method := calendarmethod.Known(raw)
		if method == "" {
			out.disputed = true
			return
		}
		if len(out.methods) > 0 && out.methods[0] != method {
			out.disputed = true
		}
		out.methods = append(out.methods, method)
	}
	if raw, ok := params["method"]; ok {
		declare(raw)
	}
	if raw, ok := bodyMethod(body); ok {
		declare(raw)
	}
	return out
}

// bodyMethod is the raw value of the first METHOD property inside a VCALENDAR.
func bodyMethod(body []byte) (string, bool) {
	inCalendar := false
	for _, line := range unfoldICalendar(body) {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		// A property name may carry parameters after a semicolon.
		name, _, _ = strings.Cut(name, ";")
		switch {
		case strings.EqualFold(name, "BEGIN") && strings.EqualFold(strings.TrimSpace(value), "VCALENDAR"):
			inCalendar = true
		case inCalendar && strings.EqualFold(name, "METHOD"):
			return value, true
		}
	}
	return "", false
}

// unfoldICalendar splits iCalendar text into logical lines. RFC 5545 folds a
// long line by a line break followed by one space or tab, which unfolding drops.
func unfoldICalendar(body []byte) []string {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n ", "")
	text = strings.ReplaceAll(text, "\n\t", "")
	return strings.Split(text, "\n")
}

// CalendarMethod is the iCalendar METHOD this message's calendar parts declared,
// resolved against the sender, or "" when they declared none.
func (m Message) CalendarMethod() string { return m.calendarMethod }
