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

// Engagement is one contact's state, in the three values a rep acts on.
type Engagement string

const (
	// EngagementAnswered — they have written back inside the window. The way in.
	EngagementAnswered Engagement = "answered"
	// EngagementNoReply — we have written and had nothing back. Following up
	// again is a decision, not a default.
	EngagementNoReply Engagement = "no_reply"
	// EngagementUntried — nobody has written to them at all. Free to approach,
	// and the most commonly missed opportunity on a stalled account.
	EngagementUntried Engagement = "untried"
)

// EngagementOf reads the state off the §4 direction counts, which are already
// folded over the same 90-day window the score uses.
//
// Inbound wins over outbound rather than being combined with it: a contact who
// replied last month and was chased again last week has both, and what decides
// the next move is that they answered. The window is the fold's, so a reply
// older than it reads as untried — deliberately, because a year-old reply is
// not a way in.
func EngagementOf(rs RelationshipStrength) Engagement {
	switch {
	case rs.Inbound90d > 0:
		return EngagementAnswered
	case rs.Outbound90d > 0:
		return EngagementNoReply
	default:
		return EngagementUntried
	}
}

// engagementOrder puts the people worth acting on first.
//
// Whoever answered leads, because they are the way in. Untried comes SECOND: on
// an account where everyone has gone quiet, the person nobody has written to is
// the only move left that is not a fourth follow-up. No-reply comes last — not
// because those contacts are worthless, but because acting on one is the move
// that needs a reason.
var engagementOrder = map[Engagement]int{
	EngagementAnswered: 0,
	EngagementUntried:  1,
	EngagementNoReply:  2,
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
