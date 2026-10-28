// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Where each contact stands with us, and the order a rep triages them in.
//
// The roster on its own answers "who works there". Before reaching out the rep
// is asking something else: who is waiting on an answer from me, whose latest
// mail have I answered, who has gone quiet on me, and who has nobody tried.
// Those are four different next moves.
//
// The rule runs on the server, not in the browser. A page is sent only some of
// an account's contacts. The company page gets 25, chosen by id. A rule in the
// browser would rank a 105-contact account from that slice. The contact worth
// writing to could then sit on a page nobody renders. Here the whole account
// is in hand.

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Engagement is one contact's state, in the five values a rep acts on.
type Engagement string

const (
	// EngagementWaiting — their latest message has no reply from us. They are
	// waiting on us, and answering is the obvious next move.
	EngagementWaiting Engagement = "waiting"
	// EngagementAnswered — we replied to their latest message. The conversation
	// is current from our side; the ball is with them.
	EngagementAnswered Engagement = "answered"
	// EngagementNoReply — we have written and had nothing back. Following up
	// again is a decision, not a default.
	EngagementNoReply Engagement = "no_reply"
	// EngagementLapsed — they have written or been written to, but not inside
	// the window. The exchange is over rather than absent, so the honest move is
	// picking it back up, not opening it.
	//
	// It exists because the alternative was a contradiction on one row: the
	// window-less last-touch date beside them said "They wrote, 1 June" while
	// the state, folded from 90-day counts, said "Not approached". Both readings
	// were right about their own input, and a rep reading them together was told
	// the contact had both written and never been in touch.
	EngagementLapsed Engagement = "lapsed"
	// EngagementUntried — nobody has written to them at all. Free to approach,
	// and the most commonly missed opportunity on a stalled account.
	EngagementUntried Engagement = "untried"
)

// EngagementOf reads the state off the strength fold. The direction counts say who
// wrote inside the 90-day window. Answered needs our mail after their latest,
// because an unprompted mail nobody answered is the account's most urgent row.
//
// When their latest message is a mail, its History badge says whether we owe
// a reply. A mail it owes nothing on, such as a calendar copy, reads answered
// only when we wrote after it.
func EngagementOf(rs RelationshipStrength) Engagement {
	replied := rs.LastInbound != nil && rs.LastOutbound != nil && rs.LastOutbound.After(*rs.LastInbound)
	switch {
	case rs.Inbound90d > 0 && rs.LastInboundMove != nil && *rs.LastInboundMove == crmcontracts.EmailSummaryMoveNeedsReply:
		return EngagementWaiting
	case rs.Inbound90d > 0 && replied:
		return EngagementAnswered
	case rs.Inbound90d > 0 && rs.LastInboundMove == nil:
		return EngagementWaiting
	case rs.Outbound90d > 0:
		return EngagementNoReply
	case rs.LastInbound != nil || rs.LastOutbound != nil:
		// Traffic, but all of it older than the window. Reported as untried this
		// was a plain falsehood beside the contact's own last-touch date.
		return EngagementLapsed
	default:
		return EngagementUntried
	}
}

// engagementOrder puts the contacts worth acting on first.
//
// Whoever is waiting on us leads, because answering them is the one move that
// is already owed. Answered comes next: that conversation is alive and worth
// keeping so. Untried comes THIRD: on an account where everyone has gone
// quiet, the contact nobody has written to is the only move left that is not a
// fourth follow-up. No-reply comes last — not because those contacts are
// worthless, but because acting on one is the move that needs a reason.
// Lapsed sits between untried and no-reply: a conversation that once ran is a
// warmer opening than a cold approach, and a colder one than a follow-up nobody
// has answered yet.
var engagementOrder = map[Engagement]int{
	EngagementWaiting:  0,
	EngagementAnswered: 1,
	EngagementUntried:  2,
	EngagementLapsed:   3,
	EngagementNoReply:  4,
}

// RankContacts sorts a contact set into triage order in place: engagement
// first, then the stronger relationship, then by id so two otherwise equal
// contacts come back in a stable order rather than the scan's.
//
// Stable rather than merely sorted, because callers page over the result: an
// unstable tie-break would let a contact appear on two pages, or on none.
func RankContacts(contacts []ContactStrength) {
	sort.SliceStable(contacts, func(i, j int) bool {
		a, b := contacts[i], contacts[j]
		ao, bo := engagementOrder[EngagementOf(a.Strength)], engagementOrder[EngagementOf(b.Strength)]
		if ao != bo {
			return ao < bo
		}
		if a.Strength.Strength != b.Strength.Strength {
			return a.Strength.Strength > b.Strength.Strength
		}
		return a.ContactID.String() < b.ContactID.String()
	})
}
