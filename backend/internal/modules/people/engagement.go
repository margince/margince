// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Where each contact stands with us, and the order a rep triages them in.
//
// The roster on its own answers "who works there". Before reaching out the rep
// is asking something else: who here has ever answered, who has gone quiet on
// me, and who has nobody tried. Those are three different next moves.
//
// This rule used to live in the browser (frontend/src/screens/coverage.ts
// reachOf), which meant it could only classify the contacts a page had already
// been sent — and the company page is sent 25 of them, chosen by id. Ranking a
// 105-contact account by a rule the server does not know is how the person
// worth writing to ends up on a page nobody renders. The rule moves here so the
// ranking happens where the whole account is in hand.

import "sort"

// Engagement is one contact's state, in the four values a rep acts on.
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
	// EngagementUntried — nobody has written to them at all. Free to approach,
	// and the most commonly missed opportunity on a stalled account.
	EngagementUntried Engagement = "untried"
)

// EngagementOf reads the state off the §4 fold: the direction counts say
// whether each side has written inside the 90-day window, and the two last-touch
// dates say who wrote LAST — which is what separates answered from waiting.
//
// Answered requires an outbound AFTER their latest inbound, not merely both
// directions having traffic: a contact whose only mail arrived unprompted has
// inbound and no reply from us, and reporting that as answered dresses the
// account's most urgent row up as a success. The window is the fold's, so a
// conversation older than it reads as untried — deliberately, because a
// year-old exchange is not a way in.
//
// The account-level state strip answers the sibling question ("whose move is
// the ACCOUNT") with its own age thresholds; this is per contact and has none,
// because a mail from yesterday is already theirs to be answered.
func EngagementOf(rs RelationshipStrength) Engagement {
	switch {
	case rs.Inbound90d > 0:
		if rs.LastInbound != nil && rs.LastOutbound != nil && rs.LastOutbound.After(*rs.LastInbound) {
			return EngagementAnswered
		}
		return EngagementWaiting
	case rs.Outbound90d > 0:
		return EngagementNoReply
	default:
		return EngagementUntried
	}
}

// engagementOrder puts the people worth acting on first.
//
// Whoever is waiting on us leads, because answering them is the one move that
// is already owed. Answered comes next: that conversation is alive and worth
// keeping so. Untried comes THIRD: on an account where everyone has gone
// quiet, the person nobody has written to is the only move left that is not a
// fourth follow-up. No-reply comes last — not because those contacts are
// worthless, but because acting on one is the move that needs a reason.
var engagementOrder = map[Engagement]int{
	EngagementWaiting:  0,
	EngagementAnswered: 1,
	EngagementUntried:  2,
	EngagementNoReply:  3,
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
		return a.PersonID.String() < b.PersonID.String()
	})
}
