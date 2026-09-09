// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package meetingmap is the pure calendar-event → meeting-activity mapping: no
// provider handle, no I/O. It is the calendar analogue of capture/mailmap — the
// test-guarded surface a calendar connector's Sync and Normalize compose, so the
// classification (all-internal skip, a cancelled meeting CLOSED rather than
// dropped, booked rooms are not guests) and the field mapping are proven by
// fixtures rather than by a live calendar. What each event settles as, and why,
// is settlement.go beside this.
//
// It lived inside the gcal package while gcal was the only calendar connector,
// which is what ADR-0054 §3 asks for — flat by default, grow a subpackage when a
// second concrete caller appears. The Microsoft 365 calendar connector is that
// second caller, and these rules are not Google's: whether a booked room counts
// as a guest, whether a meeting with no outside party is worth logging, and
// which addresses the writer judges against the workspace's own domains are
// answers about MEETINGS. Two copies would be two answers, and they would
// diverge on the case that matters — the one where a workspace's own colleagues
// silently become visible as customer touches.
//
// Each connector still owns its own decode: Google and Microsoft describe the
// same meeting with different JSON, and Event is where the two agree.
package meetingmap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// MaxBodyLen caps the stored meeting body — the timeline needs a legible
// summary of who/what, not a multi-kilobyte agenda paste. Exported because a
// connector's own fixtures assert against the cap, and a second literal in a
// test is a second answer to the same question.
const MaxBodyLen = 8000

// Event is one calendar event as any provider describes it, after that
// provider's own decode. It carries what the rules below read and nothing more:
// the raw original reaches the Sink verbatim as evidence, so a field absent
// here is not a field lost.
type Event struct {
	ID        string
	Cancelled bool
	// OwnerDeclined says the connected account answered NO to this invitation.
	// It is a different fact from Cancelled — the organizer's event is alive and
	// everyone else is still going — but it means the same thing for the seat
	// whose calendar this is, so Settlement folds the two together.
	OwnerDeclined bool
	Subject       string
	Description   string
	// StartsAt is the event's start as an instant. The zero value is an
	// unreadable start, which the Sink then stamps with capture time rather than
	// sorting the row to the beginning of history — so a decoder that cannot
	// read a start leaves this zero rather than guessing.
	StartsAt  time.Time
	Organizer Actor
	Attendees []Actor
}

// Actor is one organizer or attendee. The address is all the rules need to
// resolve the counterparty and the internal-vs-external classification; Room
// says the provider marked this a booked room or device rather than a person,
// which each decoder answers in its own vendor's terms.
type Actor struct {
	Email string
	// Name is what the invitation called this party, or "" when the provider
	// named nobody. It is the only full name an attendee-only contact ever
	// gets: a person minted from a bare invitation address is otherwise named
	// by the local part of their own email.
	Name string
	Room bool
}

// Meeting is the classified result of reading one event against the connected
// account's owner. The owner's own domain gives the internal floor; the
// workspace's registered domains (CAP-DDL-1) are the authority and are applied
// by the writer, over the full party set this reports.
type Meeting struct {
	id            string
	subject       string
	body          string
	occurredAt    time.Time
	cancelled     bool
	ownerDeclined bool
	hasExternal   bool // any party outside the OWNER's own domain — the floor, see Settle
	addresses     []string
	participants  []connector.MessageParticipant
}

// Classify applies the meeting rules to one decoded event against the account
// owner (whose domain marks "internal"). Pure — the caller has already decoded
// the bytes — so the whole mapping is fixture-provable.
func Classify(ev Event, owner string) Meeting {
	ownerDom := domainOf(owner)
	attendeeEmails, external := classifyAttendees(ev.Attendees, ownerDom)
	organizerDom := domainOf(ev.Organizer.Email)

	return Meeting{
		id:            strings.TrimSpace(ev.ID),
		subject:       strings.TrimSpace(ev.Subject),
		body:          buildBody(ev, attendeeEmails),
		occurredAt:    ev.StartsAt,
		cancelled:     ev.Cancelled,
		ownerDeclined: ev.OwnerDeclined,
		// The organizer counts as a party: an externally-organized meeting is a
		// customer touch even when the owner is the only listed attendee.
		//
		// An organizer whose address has no parseable domain counts as EXTERNAL,
		// the same rule classifyAttendees applies below: unknown is not
		// internal. Reading it as internal instead would drop a real
		// multi-party meeting on the strength of a malformed address.
		hasExternal:  external > 0 || organizerIsExternal(ev.Organizer.Email, organizerDom, ownerDom),
		addresses:    eventAddresses(ev, owner),
		participants: meetingParties(ev, strings.ToLower(strings.TrimSpace(owner))),
	}
}

// Participants are the organizer and attendees as structured rows — read by the
// replay pass that recovers meetings captured before participants were recorded.
func (m Meeting) Participants() []connector.MessageParticipant { return m.participants }

// meetingParties returns the organizer and attendees as participant rows,
// excluding the account owner.
//
// The owner is excluded because capture stamps them separately from the
// connection that produced the event — that row carries their user_id, which is
// what the interaction graph joins on, whereas a row built from this header
// would carry only an address and join nothing.
//
// Organizer wins over attendee when the same address holds both, which is the
// common case for a meeting somebody scheduled and then attended: organizing is
// the stronger statement about their part in it.
func meetingParties(ev Event, ownerLower string) []connector.MessageParticipant {
	seen := map[string]bool{}
	if ownerLower != "" {
		seen[ownerLower] = true
	}

	var out []connector.MessageParticipant
	add := func(email, name, role string) {
		address := strings.ToLower(strings.TrimSpace(email))
		if address == "" || seen[address] {
			return
		}
		seen[address] = true
		out = append(out, connector.MessageParticipant{
			Email: address, Role: role, DisplayName: strings.TrimSpace(name),
		})
	}
	add(ev.Organizer.Email, ev.Organizer.Name, connector.ParticipantRoleOrganizer)
	for _, a := range ev.Attendees {
		if a.Room {
			continue
		}
		add(a.Email, a.Name, connector.ParticipantRoleAttendee)
	}

	return connector.CapParticipants(out)
}

// ID is the provider's event id — the idempotency source id a calendar
// connector keys on (ACT-DDL-1: capture key is the event id per workspace).
func (m Meeting) ID() string { return m.id }

// ToRecord builds the provenance-stamped meeting activity for connectorName.
// The organizer and attendees are folded into a compact header on the body —
// the activity schema has no participant column, and the timeline needs to show
// who the meeting was with (the same shape mailmap uses for From/To).
// Addresses carry every party so the ONE Sink's RC-2 personal-mail gate covers
// calendar exactly as it covers mail.
// The attendee list is attested as the provider's own: it is read back from the
// calendar over the authenticated connection of a seat that is on the event, not
// parsed out of a header a sender typed. That is what lets capture bind an
// invited colleague's user_id, which the mail rule refuses for a Cc line.
// connectorName comes from the registry that ran this connector, so no record
// can claim the attestation by describing itself.
func (m Meeting) ToRecord(connectorName string, raw []byte) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connectorName, SourceID: m.id},
		Fields: capture.ActivityFields{
			Kind:       "meeting",
			Subject:    m.subject,
			Body:       m.body,
			OccurredAt: m.occurredAt,
			// A meeting is not directional (no inbound/outbound sender).
			Direction: "",
		},
		Source:       connectorName + ":" + m.id,
		CapturedBy:   "connector:" + connectorName,
		Raw:          raw,
		Participants: m.participants,
		Addresses:    m.addresses,
	}.WithProviderAttestedParticipants(true)
}

// organizerIsExternal reports whether the organizer is a party outside the
// owner's own domain. An empty address names nobody; a nonempty one whose
// domain will not parse is external, because unknown is not internal.
func organizerIsExternal(address, organizerDom, ownerDom string) bool {
	if strings.TrimSpace(address) == "" {
		return false
	}
	return organizerDom == "" || ownerDom == "" || organizerDom != ownerDom
}

// classifyAttendees returns the attendee emails (for the body header,
// order-preserving) and the count of attendees whose domain differs from the
// owner's — the external signal behind the all-internal skip. An attendee with
// no parseable domain is treated as external (unknown ≠ internal): capturing a
// possibly-external touch beats silently dropping it. A booked room is not an
// attendee at all.
func classifyAttendees(attendees []Actor, ownerDom string) (emails []string, external int) {
	for _, a := range attendees {
		email := strings.TrimSpace(a.Email)
		if email == "" || a.Room {
			continue
		}
		emails = append(emails, email)
		if dom := domainOf(email); dom == "" || ownerDom == "" || dom != ownerDom {
			external++
		}
	}
	return emails, external
}

// buildBody folds the organizer, the attendee list, and the event description
// into the stored meeting body, bounded to a legible excerpt.
func buildBody(ev Event, attendeeEmails []string) string {
	header := "Organizer: " + orDash(strings.TrimSpace(ev.Organizer.Email))
	if len(attendeeEmails) > 0 {
		header += "\nAttendees: " + strings.Join(attendeeEmails, ", ")
	}
	body := header
	if desc := strings.TrimSpace(ev.Description); desc != "" {
		body = header + "\n\n" + desc
	}
	return truncate(body, MaxBodyLen)
}

// AllDayStart anchors an all-day date (YYYY-MM-DD) at noon UTC.
//
// An all-day date is calendar-local with no timezone; reading it as midnight
// UTC lands it on the PREVIOUS day for any zone west of UTC. Noon keeps the
// intended calendar date across the whole ±12h range of real offsets, absent a
// per-event timezone. Exported because every calendar provider has all-day
// events and the off-by-one is the same one in each.
func AllDayStart(date string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(date))
	if err != nil {
		return time.Time{}, false
	}
	return t.Add(12 * time.Hour).UTC(), true
}

// domainOf returns the lowercased domain part of an address, or "" if it
// carries no "@". It splits at the LAST "@" so a quoted local part containing
// one still yields the domain.
func domainOf(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if idx := strings.LastIndex(addr, "@"); idx >= 0 {
		return addr[idx+1:]
	}
	return ""
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
	// Back off to a rune boundary so the stored excerpt is never a broken UTF-8
	// sequence.
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// eventAddresses returns every address the event names — organizer, attendees
// and the connected owner — lowercased, deduplicated, order preserved. The
// owner is included because the internal decision is taken against the
// workspace's registered domains rather than against the owner, and a workspace
// that has not registered its own domain should not have that asserted on its
// behalf here.
func eventAddresses(ev Event, owner string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(address string) {
		address = strings.ToLower(strings.TrimSpace(address))
		if address == "" || seen[address] {
			return
		}
		seen[address] = true
		out = append(out, address)
	}
	add(ev.Organizer.Email)
	for _, a := range ev.Attendees {
		if a.Room {
			continue
		}
		add(a.Email)
	}
	add(owner)
	return out
}

// Decode turns one provider's raw event bytes into the neutral Event. Each
// calendar connector supplies its own; it is the only part of reading an event
// that is genuinely the vendor's.
//
// The owner travels in because one field cannot be read without it: the RSVP
// this event carries belongs to a particular attendee, and which of them is the
// connected account is a question only the caller can answer.
type Decode func(raw []byte, owner string) (Event, error)

// CaptureOne parses, drops, cancels, or upserts ONE raw event: the whole of
// what happens to a single event, for whichever calendar decoded it.
//
// A parse failure or a deliberate drop (solo, or inside the owner's domain) is a
// no-op; only a real Sink write fault returns a non-nil error, which stops the
// pull. A package function rather than a method so a pull holds no shared state.
//
// A cancelled or declined event reaches the sink as a CANCELLATION rather than
// as nothing. The event was capturable once — that is exactly the case where a
// meeting is sitting on somebody's schedule — and this is the only pull that
// will ever mention it again, because the provider stops listing an event once
// it is off. Dropping it here is what left a cancelled meeting reading as booked
// for good.
func CaptureOne(ctx context.Context, raw []byte, sink connector.Sink, owner, connectorName string, decode Decode) error {
	ev, err := decode(raw, owner)
	if err != nil {
		return nil //nolint:nilerr // a single unparseable event is a skip, not a fatal pull error (mirrors the mail connectors)
	}
	m := Classify(ev, owner)
	// Every settlement is named, and an unnamed one is an ERROR rather than a
	// write. This type exists to tell "write it" apart from "do not", so falling
	// through to the upsert is the one wrong direction to fail in: a settlement
	// added later and not handled here would put the event on the timeline as a
	// booked meeting, which is precisely the row this path was written to stop.
	switch _, settlement := m.Settle(); settlement {
	case SettleDrop:
		return nil
	case SettleCancel:
		return cancelCaptured(ctx, sink, m, connectorName)
	case SettleCapture:
		if _, err := sink.Upsert(ctx, m.ToRecord(connectorName, raw)); err != nil {
			if errors.Is(err, connector.ErrSkip) {
				return nil
			}
			return err
		}
		return nil
	default:
		return fmt.Errorf("meetingmap: %s settled as %d, which nothing here handles", m.ID(), settlement)
	}
}

// cancelCaptured marks a meeting this workspace already captured as cancelled.
//
// A Sink that cannot cancel is not a fault. The interface is optional by
// design (connector.MeetingCanceller), so a fixture or a future sink that
// implements only Upsert behaves exactly as this package did before: the event
// is dropped. Silently, because there is nothing for an operator to fix — a
// sink without the verb was never going to record the cancellation.
//
// The cancellation is stamped with the meeting's own START, not with now(). A
// pull that runs days later would otherwise record the meeting as having been
// called off at the moment we noticed, and every question about when bookings
// fell through would answer with the sync schedule instead of the calendar.
func cancelCaptured(ctx context.Context, sink connector.Sink, m Meeting, connectorName string) error {
	canceller, ok := sink.(connector.MeetingCanceller)
	if !ok {
		return nil
	}
	key := connector.NaturalKey{SourceSystem: connectorName, SourceID: m.id}
	if err := canceller.CancelMeeting(ctx, key, m.occurredAt); err != nil {
		if errors.Is(err, connector.ErrSkip) {
			return nil
		}
		return err
	}
	return nil
}

// NormalizeOne maps ONE raw event to its meeting activity, reporting a
// deliberate drop as an ErrSkip-wrapped refusal. Pure — the test-guarded
// surface a connector's Normalize composes.
func NormalizeOne(raw []byte, owner, connectorName string, decode Decode) ([]connector.NormalizedRecord, error) {
	ev, err := decode(raw, owner)
	if err != nil {
		return nil, err
	}
	m := Classify(ev, owner)
	if reason, drop := m.SkipReason(); drop {
		return nil, fmt.Errorf("%s: dropping %s (%s): %w", connectorName, m.ID(), reason, connector.ErrSkip)
	}
	return []connector.NormalizedRecord{m.ToRecord(connectorName, raw)}, nil
}

// ParticipantsOf reads the organizer and attendees out of one stored event
// resource — the calendar twin of mailmap.ParticipantsOf, for the replay pass
// that recovers meetings captured before participants were recorded.
func ParticipantsOf(raw []byte, owner string, decode Decode) ([]connector.MessageParticipant, error) {
	ev, err := decode(raw, owner)
	if err != nil {
		return nil, err
	}
	return Classify(ev, owner).Participants(), nil
}

// SettlementOf answers what one STORED event resource settles as, for the
// backfill that closes meetings captured before the RSVP was read.
//
// It is Settle over a stored original rather than a live pull, and it exists so
// that pass asks the SAME question live capture asks. A backfill that re-derived
// "is this meeting off" from the payload itself would be a second answer to a
// question this package already owns, and the two would part company the first
// time a provider changed how it says no.
func SettlementOf(raw []byte, owner string, decode Decode) (Settlement, error) {
	ev, err := decode(raw, owner)
	if err != nil {
		return SettleDrop, err
	}
	_, settlement := Classify(ev, owner).Settle()
	return settlement, nil
}
