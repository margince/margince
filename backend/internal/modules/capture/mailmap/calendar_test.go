// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import "testing"

// The header shape Google Calendar actually sends, taken from a real mailbox:
// From is the ORGANIZER — a real human at a real address — and the machine is
// named only in Sender. There is no Auto-Submitted, no List-* pair and no bulk
// Precedence, which is why every RFC 3834 rule this package already had let it
// through as ordinary mail the owner wrote.
func calendarInviteFixture() []byte {
	return crlf(
		"Reply-To: Lars Organizer <owner@myco.com>",
		"Sender: Google Calendar <calendar-notification@google.com>",
		"Message-ID: <calendar-9810af82-85d6-4be3-992c-9fd19c9d78c1@google.com>",
		"From: Lars Organizer <owner@myco.com>",
		"To: attendee@partner.example",
		"Subject: Invitation: Weekly sync @ Fri 5 Jun 2026 11:15am",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"You have been invited to Weekly sync.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	)
}

// An invitation names an EVENT. Its recipient is an attendee, not somebody the
// workspace corresponded with, and reading the two as the same fact turned
// every person the owner had ever invited into a contact — a spouse, a language
// teacher, and the owner's own second address among them.
func TestACalendarInviteNamesNoCounterparty(t *testing.T) {
	t.Parallel()
	msg, err := Parse(calendarInviteFixture(), "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	record := msg.ToRecord("gmail", nil)
	if got := record.Counterparty.Email; got != "" {
		t.Errorf("counterparty = %q, want none — an attendee is not a counterparty", got)
	}
	if record.Counterparty.SentByOwner() {
		t.Error("an invitation vouches for nobody: groupware composed it, not the owner")
	}
}

// The message itself is kept. Somebody's meeting is a real fact about their
// week, and losing the mail to spare a wrong contact is the worse trade — the
// attendees stay recorded as participants, which is what they are.
func TestACalendarInviteIsStillCaptured(t *testing.T) {
	t.Parallel()
	msg, err := Parse(calendarInviteFixture(), "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if reason, skip := msg.SkipReason(); skip {
		t.Errorf("SkipReason = %q, want kept — the meeting is a real fact about the owner's week", reason)
	}
	record := msg.ToRecord("gmail", nil)
	if len(record.Addresses) == 0 {
		t.Error("the invitation's addresses must still be recorded")
	}
	if record.ThreadKey == "" {
		t.Error("the invitation must still join its thread")
	}
}

// The rule reads Sender, never From. An organizer is a real human whose
// ordinary mail must be unaffected, and a rule keyed on From would refuse them
// everywhere.
func TestAPersonsOwnMailIsNotACalendarNotice(t *testing.T) {
	t.Parallel()
	// The same organizer, writing to the same address, by hand.
	plain := crlf(
		"From: Lars Organizer <owner@myco.com>",
		"To: attendee@partner.example",
		"Subject: About Friday",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <handwritten@myco.com>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Shall we move it to 10?",
		"",
	)
	msg, err := Parse(plain, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "attendee@partner.example" {
		t.Errorf("counterparty = %q, want the attendee — a person's own mail is correspondence", got)
	}
}

// An .ics a person attaches to a mail they wrote is not groupware speaking for
// them, and that message IS correspondence. Requiring the provider's own Sender
// address is what keeps the rule to messages a machine composed.
func TestAHandAttachedCalendarFileIsNotANotice(t *testing.T) {
	t.Parallel()
	withICS := crlf(
		"From: Alice Example <alice@acme.com>",
		"To: owner@myco.com",
		"Subject: Here is the invite",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <hand@acme.com>",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Attaching the invite.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	)
	msg, err := Parse(withICS, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "alice@acme.com" {
		t.Errorf("counterparty = %q, want alice — a person attaching an .ics is still writing", got)
	}
}

// A cancellation and an update carry no `method=REQUEST`, and a third of the
// invitations in the mailbox this was measured on were one or the other. A rule
// keyed on the method would have let them through.
func TestACancellationIsAlsoACalendarNotice(t *testing.T) {
	t.Parallel()
	cancelled := crlf(
		"Sender: Google Calendar <calendar-notification@google.com>",
		"From: Lars Organizer <owner@myco.com>",
		"To: attendee@partner.example",
		"Subject: Cancelled: Weekly sync",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <calendar-cancel@google.com>",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"This event has been cancelled.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=CANCEL",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	)
	msg, err := Parse(cancelled, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "" {
		t.Errorf("counterparty = %q, want none — a cancellation names an event too", got)
	}
}

// The two calendar guards do different work, and a test that passes on the
// other's is not a test.
//
// Withholding the ATTESTATION stops the invitation becoming T1 evidence: an
// attested outbound spares that address from transactional suppression later,
// so an invitation would vouch for an attendee's future bulk mail. Withholding
// the COUNTERPARTY stops a ledger question being opened about somebody who is
// merely on a meeting. Each refuses the contact on its own, so each is asserted
// on its own here.
func TestBothCalendarGuardsAreLoadBearing(t *testing.T) {
	t.Parallel()
	msg, err := Parse(calendarInviteFixture(), "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !msg.machineTouched {
		t.Error("an invitation must count as machine-touched, or it vouches for its attendee")
	}
	if !msg.calendarNotice {
		t.Error("the invitation was not recognised at all")
	}
	record := msg.ToRecord("gmail", nil)
	if record.Counterparty.SentByOwner() {
		t.Error("the attestation guard: groupware composed this, so it vouches for nobody")
	}
	if record.Counterparty.Email != "" {
		t.Error("the counterparty guard: an attendee is not somebody the workspace wrote to")
	}
}

// An invitation the owner RECEIVES is not the case this rule is about.
//
// Its organizer is named in From and is a person who wrote to the owner —
// exactly the counterparty capture exists to record. The wrong contacts all
// came from the other direction: the owner invites, and every attendee reads as
// somebody the workspace writes to. Suppressing the inbound side as well
// deleted a real correspondent from the record, and dropped the participants
// with them, because otherParties walks To/Cc/Bcc and never From.
func TestAnInviteTheOwnerRECEIVESKeepsItsOrganizer(t *testing.T) {
	t.Parallel()
	inbound := crlf(
		"Sender: Google Calendar <calendar-notification@google.com>",
		"From: Alice Organizer <alice@partner.example>",
		"To: owner@myco.com",
		"Subject: Invitation: Kickoff",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <cal-inbound@google.com>",
		"Content-Type: multipart/alternative; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"You have been invited.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	)
	msg, err := Parse(inbound, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "alice@partner.example" {
		t.Errorf("counterparty = %q, want the organizer — somebody invited the owner to something", got)
	}
}

// Some clients send the .ics as a named ATTACHMENT rather than an inline part.
// Both routes reach the file collector before the body switch, so reading the
// type only in that switch missed the attachment form entirely.
func TestAnInviteWhoseCalendarIsAnAttachmentIsStillRecognised(t *testing.T) {
	t.Parallel()
	attached := crlf(
		"Sender: Google Calendar <calendar-notification@google.com>",
		"From: Lars Organizer <owner@myco.com>",
		"To: attendee@partner.example",
		"Subject: Invitation: Weekly sync",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <cal-attached@google.com>",
		"Content-Type: multipart/mixed; boundary=\"b1\"",
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"You have been invited to Weekly sync.",
		"",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST; name=\"invite.ics\"",
		"Content-Disposition: attachment; filename=\"invite.ics\"",
		"",
		"BEGIN:VCALENDAR",
		"END:VCALENDAR",
		"",
		"--b1--",
		"",
	)
	msg, err := Parse(attached, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "" {
		t.Errorf("counterparty = %q, want none — the .ics form does not change what it is", got)
	}
}

// Exchange and Outlook name no calendar Sender at all: the organizer's own
// server sends the invitation. Content-Class is the only evidence, so it is
// read independently of the sender allowlist — gating it behind a Google
// address made this arm unreachable for the senders that actually write it.
func TestAnExchangeInviteIsRecognisedByItsContentClass(t *testing.T) {
	t.Parallel()
	exchange := crlf(
		"From: Lars Organizer <owner@myco.com>",
		"To: attendee@partner.example",
		"Subject: Weekly sync",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <exchange-invite@myco.com>",
		"Content-Class: urn:content-classes:calendarmessage",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"You have been invited.",
		"",
	)
	msg, err := Parse(exchange, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "" {
		t.Errorf("counterparty = %q, want none — Exchange says outright what this is", got)
	}
}

// Every header this rule reads is typed by whoever sent the message, so the
// question is what forging them buys. The answer is nothing, and the reason is
// the outbound scoping rather than any check on the headers themselves.
//
// A stranger who sets both `Sender:` and `Content-Class:` is still INBOUND —
// their From is not the mailbox owner, and only the owner's own address makes a
// message outbound. So they cannot talk themselves out of being recorded, which
// is the one thing a sender would want from this path.
func TestAForgedCalendarHeaderDoesNotHideASender(t *testing.T) {
	t.Parallel()
	forged := crlf(
		"Sender: Google Calendar <calendar-notification@google.com>",
		"Content-Class: urn:content-classes:calendarmessage",
		"From: Spammer <spam@bad.example>",
		"To: owner@myco.com",
		"Subject: Buy now",
		"Date: Mon, 01 Jun 2026 22:03:51 +0000",
		"Message-ID: <forged@bad.example>",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"hello",
		"",
	)
	msg, err := Parse(forged, "owner@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := msg.ToRecord("gmail", nil).Counterparty.Email; got != "spam@bad.example" {
		t.Errorf("counterparty = %q, want the sender — forged calendar headers must not hide anybody", got)
	}
}
