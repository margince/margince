// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "time"

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

	Subject    string
	Body       string
	OccurredAt time.Time
	Direction  string // connector.DirectionInbound | DirectionOutbound | "" (not directional)

	// HasCalendarPart says the message carried a text/calendar payload.
	//
	// What the PARSER can vouch for, and no more. It is not "this is an
	// invitation": ordinary mail attaches an .ics, and groupware can announce an
	// event without one. Deciding what a message asks of its reader is a
	// judgement made later, from this fact among others.
	HasCalendarPart bool
}

// LeadFields is a captured prospect bound for the lead pool — never
// directly a contact/company (ADR-0008: leads graduate, raw
// capture does not mint clean-core rows).
type LeadFields struct {
	FullName    string
	Email       string
	CompanyName string
	Title       string
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
