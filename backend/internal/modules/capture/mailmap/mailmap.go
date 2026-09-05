// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailmap is the pure RFC822 → activity mapping shared by every
// mail-capture connector (imap, gmail): no provider handle, no I/O beyond
// reading the in-memory message bytes. This is the test-guarded surface —
// a connector's Sync and Normalize compose these functions, so the
// classification (direction, skip rules) and the field mapping are proven
// by fixtures, not a live mailbox. ToRecord is parameterised by the
// connector name so the same mapping stamps whichever connector read the
// message onto the row's provenance.
package mailmap

import (
	"bytes"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-message/mail"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// maxBodyLen caps the stored email body — the timeline needs a legible
// excerpt, not the full multi-megabyte thread with quoted history.
const maxBodyLen = 8000

// Message is the pure result of reading one RFC822 message against the
// mailbox owner — everything the mapping needs, with no provider handle.
type Message struct {
	messageID        string
	subject          string
	body             string
	occurredAt       time.Time
	direction        string // inbound | outbound
	from             string
	to               string
	counterparty     string
	counterpartyName string // display name from the counterparty's header — untrusted text
	threadKey        string // conversation identity: References root / In-Reply-To / own Message-ID
	// deliveredTo is the address the receiving infrastructure recorded this
	// message as delivered to, from a position a sender could not have
	// authored, and empty whenever no such claim can be trusted
	// (deliveredto.go). It is how a forwarding alias becomes discoverable.
	deliveredTo string
	autoReply   bool // a reply nobody chose to write: kept off the timeline
	// machineTouched is the BROADER question — did any machine have a hand in
	// this? It never drops a message; it only refuses the outbound attestation,
	// so a responder's reply cannot vouch for an address the owner never chose.
	machineTouched bool
	// calendarNotice is groupware sending an invitation on a person's behalf.
	// It implies machineTouched and says something narrower: this message names
	// an EVENT, so its recipients are attendees rather than people the workspace
	// corresponded with.
	calendarNotice bool
	// hasCalendarPart is the raw evidence calendarNotice is derived from: this
	// message carried a text/calendar payload. Kept in its own right because
	// calendarNotice answers only the OUTBOUND contact-minting question, and an
	// inbound invitation is a fact about the message either way.
	hasCalendarPart bool
	listUnsubscribe bool // an RFC 2369 List-Unsubscribe header — transactional-gate corroboration
	sentByOwner     bool // the PROVIDER attested the owner sent this — set by AttestSentByOwner, never parsed
	// participants are everyone on To, Cc and Bcc who is neither the mailbox
	// owner nor the counterparty — the two ends already have their own rows.
	participants []connector.MessageParticipant
	// addresses is every address the message names, the two ends included. The
	// internal-vs-external rule is about the whole message, so it needs the
	// full set rather than the derived ends (ADR-0082 §3).
	addresses []string
	// parts are the files the message carried, already bounded, sanitized and
	// sniffed. partDrops names the ones the bounds refused, so a message whose
	// files were dropped never reads as a message with no files (DOC-AC-12).
	parts     []Part
	partDrops []PartDrop
}

// AttestSentByOwner returns a copy carrying the provider's own attestation
// that the authenticated mailbox owner sent this message — Gmail's SENT
// label, an IMAP \Sent special-use mailbox, Microsoft's SentItems folder.
// The signal cannot come from Parse: every header this package reads is
// attacker-controlled, and the T1 correspondence-positive gate (ADR-0072 §1)
// treats a sent message as affirmative intent toward its recipient. Only a
// connector holding an authenticated provider handle can vouch for it.
func (m Message) AttestSentByOwner(sent bool) Message {
	// A machine-touched message never attests, however the provider filed it.
	// An autoresponder's reply IS genuinely owner-authored and genuinely in
	// Sent, so nothing downstream could tell it from correspondence the owner
	// chose — and it would spare an address the owner never chose to write to
	// (ADR-0072 residual (b)).
	m.sentByOwner = sent && !m.machineTouched
	return m
}

// Counterparty is the non-owner address on the message (the person this
// mail was with) — exported so a connector can tally distinct contacts.
func (m Message) Counterparty() string { return m.counterparty }

// ThreadKey is the conversation identity this message belongs to.
func (m Message) ThreadKey() string { return m.threadKey }

// Parse reads the headers and the text body of one message and classifies
// its direction relative to the mailbox owner.
func Parse(raw []byte, owner string) (Message, error) {
	reader, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return Message{}, fmt.Errorf("mailmap: parsing message: %w", err)
	}
	header := reader.Header

	messageID, _ := header.MessageID()
	subject, _ := header.Subject()
	occurredAt, _ := header.Date()

	fromList, _ := header.AddressList("From")
	toList, _ := header.AddressList("To")
	// A malformed Cc line yields no addresses rather than failing the message:
	// the mail is already read off the wire, and losing the CCs is a smaller
	// loss than dropping the correspondence.
	ccList, _ := header.AddressList("Cc")
	// Bcc survives only on the sender's OWN copy — a recipient's copy never
	// carries it. Where it does survive it is a real party to the message, and
	// the internal-vs-external decision has to see it: a colleague who blind-
	// copied a customer wrote to a customer. Where it does not survive, the
	// message reads as one address short, which is the accepted loss named in
	// ADR-0082 §3 and not something this parser can recover.
	bccList, _ := header.AddressList("Bcc")
	deliveredTo := TopDeliveredTo(header)
	from := firstAddress(fromList)
	to := firstAddress(toList)

	body, parts, drops, hasCalendarPart := extractText(reader)

	ownerLower := strings.ToLower(strings.TrimSpace(owner))
	direction := connector.DirectionInbound
	counterparty := from
	counterpartyName := displayName(fromList, counterparty)
	if strings.ToLower(from) == ownerLower && ownerLower != "" {
		direction = connector.DirectionOutbound
		counterparty = firstNonOwner(toList, ownerLower)
		counterpartyName = displayName(toList, counterparty)
	}

	autoSubmitted, precedence := header.Values("Auto-Submitted"), header.Values("Precedence")
	autoReply := isAutoReply(autoSubmitted, precedence)
	machineTouched := isMachineTouched(autoSubmitted, precedence, hasMachineHandledHeader(header))
	// Groupware speaking for a person is a machine having a hand in the message,
	// and it is the one shape no RFC 3834 marker covers: an invitation carries
	// no Auto-Submitted, no List-* pair and no bulk Precedence. Withholding the
	// attestation here is a DIFFERENT effect from withholding the counterparty
	// in recordCounterparty — see that function — and each refuses the contact
	// on its own, so each carries its own test.
	// Only an invitation the OWNER sent. An invitation the owner RECEIVES names
	// its organizer in From, and that organizer is a person who wrote to them —
	// suppressing them would delete a real counterparty from the record, which
	// is the opposite of this rule's purpose. The wrong contacts came from the
	// outbound direction: the owner invites, and every attendee reads as
	// somebody the workspace writes to.
	calendarNotice := direction == connector.DirectionOutbound &&
		calendarNotification(header, hasCalendarPart)
	if calendarNotice {
		machineTouched = true
	}

	return Message{
		messageID:        strings.TrimSpace(messageID),
		subject:          strings.TrimSpace(subject),
		body:             body,
		occurredAt:       occurredAt,
		direction:        direction,
		from:             from,
		to:               to,
		counterparty:     counterparty,
		counterpartyName: counterpartyName,
		threadKey:        threadKey(header.Get("References"), header.Get("In-Reply-To"), messageID),
		deliveredTo:      deliveredTo,
		autoReply:        autoReply,
		machineTouched:   machineTouched,
		calendarNotice:   calendarNotice,
		hasCalendarPart:  hasCalendarPart,
		listUnsubscribe:  strings.TrimSpace(header.Get("List-Unsubscribe")) != "",
		participants:     otherParties(toList, ccList, bccList, ownerLower, participantExclusion(counterparty, calendarNotice)),
		addresses:        allAddresses(fromList, toList, ccList, bccList),
		parts:            parts,
		partDrops:        drops,
	}, nil
}

// Addresses is every address this message names — From, To, Cc and whatever Bcc
// survived — deduplicated and lowercased, including the mailbox owner's own.
//
// It exists for the internal-vs-external decision, which is a question about
// the WHOLE message and so cannot be answered from the derived counterparty:
// that is one end of the exchange, chosen for direction, and a message is only
// internal when every party to it is. The owner is included rather than assumed
// internal — their own domain is usually registered, but a workspace that has
// not registered it should not have that fact invented here.
func (m Message) Addresses() []string { return m.addresses }

// ParticipantsOf reads the further parties out of one stored original.
//
// The replay pass calls it for messages captured before participants were
// recorded. It is a narrow seam on purpose: the pass wants exactly the CC and
// To names, and giving it Parse's whole Message would invite it to re-derive
// direction or subject from headers the activity row already settled at
// capture time.
func ParticipantsOf(raw []byte, owner string) ([]connector.MessageParticipant, error) {
	msg, err := Parse(raw, owner)
	if err != nil {
		return nil, err
	}
	return msg.participants, nil
}

// otherParties returns everyone on To and Cc who is neither the mailbox owner
// nor the counterparty.
//
// Both exclusions matter and for different reasons. The owner and the
// counterparty are the two ends of the exchange and already get their own
// rows, stamped from the connection rather than from a header — a second row
// for either would either collide with the uniqueness index or, worse, record
// the same human twice under two roles.
//
// To wins over Cc when an address appears on both, which is a real thing
// senders do: a direct recipient who is also copied was addressed directly,
// and that is the stronger claim about their part in the conversation.
func otherParties(toList, ccList, bccList []*mail.Address, ownerLower, counterparty string) []connector.MessageParticipant {
	counterpartyLower := strings.ToLower(strings.TrimSpace(counterparty))
	seen := map[string]bool{ownerLower: true, counterpartyLower: true}
	delete(seen, "")

	var out []connector.MessageParticipant
	add := func(list []*mail.Address, role string) {
		for _, a := range list {
			address := strings.ToLower(strings.TrimSpace(a.Address))
			if address == "" || seen[address] {
				continue
			}
			seen[address] = true
			out = append(out, connector.MessageParticipant{Email: address, Role: role})
		}
	}
	add(toList, connector.ParticipantRoleTo)
	add(ccList, connector.ParticipantRoleCC)
	// Bcc last, and so weakest of the three: an address that was also addressed
	// openly was addressed openly, whatever else the sender did with it.
	add(bccList, connector.ParticipantRoleBCC)

	return connector.CapParticipants(out)
}

// allAddresses returns every address the message names across From, To, Cc and
// Bcc — lowercased, deduplicated, order preserved. Unlike otherParties it
// excludes nobody: the internal decision is about the whole message, and the
// owner and counterparty are as much a part of it as the copies.
func allAddresses(lists ...[]*mail.Address) []string {
	seen := make(map[string]bool)
	var out []string
	for _, list := range lists {
		for _, a := range list {
			address := strings.ToLower(strings.TrimSpace(a.Address))
			if address == "" || seen[address] {
				continue
			}
			seen[address] = true
			out = append(out, address)
		}
	}
	return out
}

// threadKey derives the conversation identity from the standard reply
// headers: the References ROOT (its first id — stable across every reply in
// the thread), else In-Reply-To, else the message's own id (a fresh thread
// is rooted at its opener, so later replies referencing it join it). Never
// a subject heuristic — "Re: Invoice" joining unrelated threads is worse
// than no join (CAP-FORMULA-1's no-subject-fallback rule).
func threadKey(references, inReplyTo, messageID string) string {
	if refs := strings.Fields(references); len(refs) > 0 {
		return trimAngle(refs[0])
	}
	if irt := strings.TrimSpace(inReplyTo); irt != "" {
		return trimAngle(irt)
	}
	return trimAngle(strings.TrimSpace(messageID))
}

// trimAngle strips the RFC822 angle brackets off a message id.
func trimAngle(id string) string {
	return strings.TrimSuffix(strings.TrimPrefix(id, "<"), ">")
}

// displayName returns the header display name for addr from list, "" when
// the header carried none. The value is whatever the sender typed — hostile
// input until a consumer sanitizes it.
func displayName(list []*mail.Address, addr string) string {
	for _, a := range list {
		if strings.EqualFold(a.Address, addr) {
			return strings.TrimSpace(a.Name)
		}
	}
	return ""
}

// ID is the RFC822 Message-ID — the idempotency source id every mail
// connector keys on (data-model §7/§8).
func (m Message) ID() string { return m.messageID }

// ToRecord builds the provenance-stamped activity record for the connector
// named connectorName (e.g. "imap", "gmail"): NaturalKey.SourceSystem and
// the Source/CapturedBy prefixes all carry that name, so the same message
// read over a different transport is still deduped on (name, Message-ID).
// The counterparty (From/To) is folded into a compact header on the body —
// the activity schema has no dedicated participant column, and the timeline
// needs to show who the mail was with.
func (m Message) ToRecord(connectorName string, raw []byte) connector.NormalizedRecord {
	source := connectorName + ":" + m.messageID
	header := fmt.Sprintf("From: %s\nTo: %s", orDash(m.from), orDash(m.to))
	body := header
	if m.body != "" {
		body = header + "\n\n" + m.body
	}
	body = truncate(body, maxBodyLen)

	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connectorName, SourceID: m.messageID},
		Fields: capture.ActivityFields{
			Kind:            "email",
			Subject:         m.subject,
			Body:            body,
			OccurredAt:      m.occurredAt,
			Direction:       m.direction,
			HasCalendarPart: m.hasCalendarPart,
		},
		Source:       source,
		CapturedBy:   "connector:" + connectorName,
		Raw:          raw,
		DeliveredTo:  m.deliveredTo,
		Counterparty: m.recordCounterparty(),
		ThreadKey:    m.threadKey,
		Participants: m.participants,
		Addresses:    m.addresses,
		Parts:        m.recordParts(),
		PartDrops:    m.recordDrops(),
	}
}

// domainOf returns the lowercased domain part of an address, or "" if the
// address carries no "@". It splits at the LAST "@" so a quoted local part
// containing one (e.g. `"weird@local"@example.com`) still yields the domain.
func domainOf(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if idx := strings.LastIndex(addr, "@"); idx >= 0 {
		return addr[idx+1:]
	}
	return ""
}

func firstAddress(list []*mail.Address) string {
	if len(list) == 0 {
		return ""
	}
	return list[0].Address
}

func firstNonOwner(list []*mail.Address, ownerLower string) string {
	for _, a := range list {
		if strings.ToLower(a.Address) != ownerLower {
			return a.Address
		}
	}
	return firstAddress(list)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	// Back off to a rune boundary so the stored excerpt is never a broken
	// UTF-8 sequence.
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// recordParts hands the collected files to the seam.
//
// The two shapes stay separate types, and the line between them is a stage
// rather than an owner: mailmap.Part is a candidate MID-WALK, which is why the
// collector can still refuse one, and connector.Part is a file that already
// cleared every bound. The copy below is where that transition is made, so a
// part cannot reach the seam without passing through it.
//
// This is NOT the "second spelling" that pkg/extension/files.go rules out. That
// rule is about two producers each declaring their own file type and therefore
// their own bounds; here there is one producer, one set of bounds, and one
// published file type at the seam that every producer shares.
func (m Message) recordParts() []connector.Part {
	if len(m.parts) == 0 {
		return nil
	}
	out := make([]connector.Part, 0, len(m.parts))
	for _, part := range m.parts {
		out = append(out, connector.Part{
			Ordinal:      part.Ordinal,
			Filename:     part.Filename,
			ContentType:  part.ContentType,
			DeclaredType: part.DeclaredType,
			Body:         part.Body,
		})
	}
	return out
}

func (m Message) recordDrops() []connector.PartDrop {
	if len(m.partDrops) == 0 {
		return nil
	}
	out := make([]connector.PartDrop, 0, len(m.partDrops))
	for _, drop := range m.partDrops {
		out = append(out, connector.PartDrop{Reason: drop.Reason, Count: drop.Count})
	}
	return out
}
