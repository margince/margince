// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// PO-F-1's EXACT tier: the identifying keys a candidate carries, what each one
// reaches, and how routing picks between them. The fuzzy tier next door scores
// similarity; nothing here does — a key either names a live record or it does
// not, and the only judgement is which lane speaks first.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LaneConflict names both sides of an exact-lane disagreement and which
// lane spoke for each, so the caller's policy has the evidence it needs
// without re-running the ladder.
//
// RoutedLane == RivalLane means ONE lane named both: the candidate's own keys
// disagree about who it is. A card carrying two addresses that belong to two
// contacts used to resolve to whichever had the lower id, as a single exact
// hit at confidence 1, with nothing anywhere saying the choice was arbitrary.
type LaneConflict struct {
	RoutedTo, Rival       ids.ContactID
	RoutedLane, RivalLane string
}

// SplitWithinLane reports whether this conflict is one lane disagreeing with
// itself rather than two lanes disagreeing with each other. Both are the same
// question for a reviewer — which of these two records is it — and the
// difference is what the evidence says, so it is a reader on the struct rather
// than a second type.
func (c LaneConflict) SplitWithinLane() bool { return c.RoutedLane == c.RivalLane }

// The exact lanes, named for LaneConflict's evidence. Ladder order is
// routing precedence: an established channel binding outranks a shared
// address, which outranks a phone number households and switchboards
// share.
//
// LaneEmail alone is exported, and only because the published
// extension.MergeKeyEmail must equal it: a source declares that key to have an
// address reach this lane, and a fitness test outside this package reads both to
// hold them equal. The other two name no vocabulary beyond this module.
const (
	laneChannelIdentity = "channel_identity"
	LaneEmail           = "email"
	lanePhone           = "phone"
)

// exactLane is one lane's answer, in ladder order.
type exactLane struct {
	name string
	// contactIDs are the DISTINCT contacts this lane's keys reach, lowest id
	// first and at most two — see exactContactIDs for why two.
	contactIDs []ids.ContactID
}

func (l exactLane) found() bool { return len(l.contactIDs) > 0 }

// routed is the contact this lane speaks for: the lowest id, which is the same
// answer the `ORDER BY … LIMIT 1` this replaced gave.
func (l exactLane) routed() ids.ContactID { return l.contactIDs[0] }

// selfRival is the second owner this lane named, and reports whether there was
// one. A lane that answers two contacts routed to one of them by id order and
// discarded the other — the discarded one is the whole subject of this
// method: it is a fact about the payload that nothing recorded.
func (l exactLane) selfRival() (ids.ContactID, bool) {
	if len(l.contactIDs) < 2 {
		return ids.ContactID{}, false
	}
	return l.contactIDs[1], true
}

// exactLanes runs every exact lane, in ladder order. All of them run even
// once one has hit: a disagreement between two lanes is itself an answer
// the caller needs, and only the rival lanes can report it. A lane whose
// candidate keys are empty costs no query.
func exactLanes(ctx context.Context, tx pgx.Tx, c ContactCandidate) ([]exactLane, error) {
	channelHits, err := exactContactByChannelIdentity(ctx, tx, c.ChannelIdentities)
	if err != nil {
		return nil, err
	}
	emailHits, err := exactContactByEmail(ctx, tx, c.Emails)
	if err != nil {
		return nil, err
	}
	phoneHits, err := exactContactByPhone(ctx, tx, c.Phones)
	if err != nil {
		return nil, err
	}
	return []exactLane{
		{laneChannelIdentity, channelHits},
		{LaneEmail, emailHits},
		{lanePhone, phoneHits},
	}, nil
}

// routeExact picks the routed contact deterministically — the first lane
// that hit — and reports the first later lane that named someone else.
// Routing is immediate and never deferred to a human: a message with
// nowhere to land is worse than a message on the record whose binding was
// established first.
func routeExact(lanes []exactLane) (ContactResolution, bool) {
	for i, lane := range lanes {
		if !lane.found() {
			continue
		}
		return ContactResolution{
			Decision:    DecisionExactCollision,
			ContactID:   lane.routed(),
			MatchedLane: lane.name,
			Conflict:    rivalOf(lane, lanes[i+1:]),
		}, true
	}
	return ContactResolution{}, false
}

// rivalOf finds the one contact to report alongside the routed one.
//
// A LATER LANE FIRST, which is the order this has always reported in and is
// deliberately unchanged: a case that raised a review row before raises the
// same row now. The routed lane's own second owner is reported only when no
// later lane disagreed — so this is additive, and the only candidates whose
// report changes are the ones that had none.
//
// One rival, not a set: the review it feeds is a PAIR of records for a human to
// judge. A third owner would be a second pair, and the next message from the
// same payload raises it once this one is resolved.
func rivalOf(routed exactLane, later []exactLane) *LaneConflict {
	for _, lane := range later {
		if lane.found() && lane.routed() != routed.routed() {
			return &LaneConflict{
				RoutedTo: routed.routed(), Rival: lane.routed(),
				RoutedLane: routed.name, RivalLane: lane.name,
			}
		}
	}
	if rival, split := routed.selfRival(); split {
		return &LaneConflict{
			RoutedTo: routed.routed(), Rival: rival,
			RoutedLane: routed.name, RivalLane: routed.name,
		}
	}
	return nil
}

// exactContactByEmail is PO-F-1 tier 1. Every candidate email is checked;
// the lowest contact id wins so a candidate colliding on two emails
// against two contacts resolves the same way on every run.
func exactContactByEmail(ctx context.Context, tx pgx.Tx, emails []string) ([]ids.ContactID, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	lowered := make([]string, 0, len(emails))
	for _, e := range emails {
		lowered = append(lowered, normalizeEmail(e))
	}
	return exactContactIDs(ctx, tx, `
		SELECT DISTINCT contact_id FROM contact_email
		WHERE email = ANY($1) AND archived_at IS NULL
		ORDER BY contact_id
		LIMIT 2`, lowered)
}

// exactContactIDs collects what one exact lane's keys reach: the distinct
// contacts, lowest id first.
//
// AT MOST TWO, and the caller's statement says so with its own LIMIT. Two is
// the whole answer a lane owes. The first is the contact it routes to — the
// same lowest id the `LIMIT 1` this replaced returned, which is what makes the
// routing outcome provably unchanged — and the second is the evidence that the
// candidate's own keys named more than one owner. The report built from it
// names ONE rival, so a third owner adds nothing a review row can carry, and
// the cap is what keeps a payload carrying a hundred addresses from deciding
// how much this query returns.
func exactContactIDs(ctx context.Context, tx pgx.Tx, statement string, args ...any) ([]ids.ContactID, error) {
	rows, err := tx.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("dedupe contact exact tier: %w", err)
	}
	defer rows.Close()
	var out []ids.ContactID
	for rows.Next() {
		var id ids.ContactID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("dedupe contact exact tier: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dedupe contact exact tier: %w", err)
	}
	return out, nil
}
