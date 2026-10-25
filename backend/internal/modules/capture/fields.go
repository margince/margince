// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"math"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The typed Fields payloads a NormalizedRecord carries (the port keeps
// Fields as any so the seam stays leaf-pure; the sink switches on these
// concrete shapes and a wrong mapping fails loudly, not silently).

// ActivityFields is a captured interaction bound for the timeline.
type ActivityFields struct {
	Kind string // email | call | meeting | note | task | message

	// ChannelProvider names the messaging transport that carried this record —
	// a channel_provider row — and is empty for anything that did not arrive on
	// one (a mail capture, a meeting, a note).
	//
	// It is a separate field from Kind because they answer separate questions:
	// Kind is what sort of interaction happened, ChannelProvider is how it
	// travelled. They were one column for as long as the only channels were
	// named as kinds, and the send path recovered the transport by reading a
	// kind back as a provider name — which stops being possible the moment a
	// provider that is not also a kind exists.
	ChannelProvider string

	Subject         string
	Body            string
	OccurredAt      time.Time
	DurationSeconds *int
	Direction       string // connector.DirectionInbound | DirectionOutbound | "" (not directional)

	// HasCalendarPart says the message carried a text/calendar payload.
	//
	// What the PARSER can vouch for, and no more. It is not "this is an
	// invitation": ordinary mail attaches an .ics, and groupware can announce an
	// event without one. Deciding what a message asks of its reader is a
	// judgement made later, from this fact among others.
	HasCalendarPart bool
	// CalendarMethod is that part's RFC 5546 METHOD, or "" when none is known.
	// REPLY, COUNTER and DECLINECOUNTER answer an invitation rather than ask.
	CalendarMethod string
	// BodyCompleteness says how much of a mail's text the parser decoded. Zero
	// for anything not parsed from a mail, which is stored as complete.
	BodyCompleteness BodyCompleteness
	// CredentialsMasked says masking changed this record's subject or body. Only
	// then may equal text fail to prove one message (replaymaskedproof.go).
	CredentialsMasked bool

	// SentByColleague is the live seat who wrote a mail this mailbox holds a
	// copy of (asSentByColleagueTx); zero for every other record.
	SentByColleague ids.UUID

	// Envelope is what the mail parser read about addressing and links; nil
	// for anything not parsed from a mail, and for list mail.
	Envelope *MailEnvelope

	// judged is what the sink concluded from Envelope. Unexported so no
	// connector can state the conclusion for it.
	judged envelopeFacts
}

// MailEnvelope is what one mail's headers and body say, before any judgement.
type MailEnvelope struct {
	// DirectRecipients preserves To separately from copied readers.
	DirectRecipients []string
	// OpenRecipients holds the To and Cc addresses, the mailbox owner's included.
	// Empty for a mail sent to undisclosed recipients.
	OpenRecipients []string
	// RecipientsUnreadable says a To or Cc header would not parse, so
	// OpenRecipients may miss the reader's own address.
	RecipientsUnreadable bool
	// LinkHosts are the hosts the body's web links lead to, with a known
	// redirect wrapper already unwrapped.
	LinkHosts []string
	// Links are the web addresses behind LinkHosts, in first-seen order, with
	// the text a reader sees on each. They come from the same walk as LinkHosts.
	Links []MailLink
	// LinksTruncated says the parser stopped before it read every link, so
	// LinkHosts may miss one.
	LinksTruncated bool
}

// MailLink is one web address a mail body links to and the text shown on it,
// empty for an address written out in plain text.
type MailLink struct {
	URL  string
	Text string
}

// LeadFields is a captured prospect bound for the lead pool — never
// directly a contact/company (ADR-0008: leads graduate, raw
// capture does not mint clean-core rows).
type LeadFields struct {
	FullName    string
	Email       string
	CompanyName string
	Title       string
	// Origin is where the prospect came from, a lead_origin key, and empty for a
	// connector that carries none. The table's foreign key refuses an unknown
	// key, so the sink needs no catalog of its own. OriginDetail is the
	// connector's own wording beside it.
	Origin       string
	OriginDetail string
	// Unowned declines ownership on the granting human's behalf, for a capture
	// that is a RECORD rather than an assignment.
	//
	// The default is the opposite and stays that way: a captured lead belongs to
	// the human whose connector found it, because the connector's own replay is
	// a write and an ownerless row is nobody's to change (auth.EnsureWritable
	// refuses one), so a lead it created it could never resume.
	//
	// This is for the path with no replay behind it. A name read off a company
	// website and accepted by a human is a record that the contact exists, not a
	// statement that the accepter has taken them on: owning it would put them in
	// that contact's "owes a reply" lane and start their first-response clock,
	// for somebody who has never written to anyone.
	Unowned bool
}

// StorableDuration is seconds as activity.duration_seconds can hold it, or nil
// when it is negative or past that int4 column. An event a sender wrote to run
// for a century would otherwise fail the insert, and with it the whole sync.
func StorableDuration(seconds *int) *int {
	if seconds == nil || *seconds < 0 || *seconds > math.MaxInt32 {
		return nil
	}
	return seconds
}

// BodyCompleteness is how much of a mail's text the parser could decode.
type BodyCompleteness string

// The stored values of activity.body_completeness.
const (
	BodyComplete BodyCompleteness = "complete"
	// BodyPartial is a mail with a text part that was not read in full. The
	// part broke while decoding, or it was cut at the per-part limit.
	BodyPartial BodyCompleteness = "partial"
	// BodyUnreadable is a mail whose Body holds only the From/To lines.
	BodyUnreadable BodyCompleteness = "unreadable"
	// BodyForwarded is a mail that said nothing of its own, whose Body is the
	// text of the original it forwarded. Those are not its sender's words.
	BodyForwarded BodyCompleteness = "forwarded"
)

func (c BodyCompleteness) stored() BodyCompleteness {
	if c == "" {
		return BodyComplete
	}
	return c
}
