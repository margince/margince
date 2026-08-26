// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// What a deal's silence actually consists of.
//
// The nightly close-date sweep downgrades a deal that has gone quiet and asks a
// human whether it is still alive. That question is unanswerable from the word
// "quiet": the reader needs to know which way the silence runs. A deal where the
// customer wrote and nobody answered is a dropped ball on our side; a deal where
// we wrote and got nothing back is a prospect going cold. They call for opposite
// actions, and both used to arrive as the same sentence.
//
// So this reads the last message in each direction, and who was on the far end
// of it — under whatever principal the caller bound, which for the sweep is the
// DEAL'S OWNER rather than its own system identity. The activity discover
// clause below is what makes that choice bite: without it every reader would
// get the same rows and the principal would be decoration.
//
// Names are deliberately NOT resolved here. This module cannot import people,
// and the composition root attaches them inside the same owner-bound
// transaction through PersonNamesTx, which carries the person object gate a
// local copy would drop.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// QuietSide is the last message in one direction on a deal, and the
// counterparty on it.
type QuietSide struct {
	At time.Time
	// Kind is the activity kind the message was — an email, a call, a meeting.
	// The reason says what actually happened, and "wrote" on a phone call is a
	// small lie that costs the sentence its authority.
	Kind string
	// PersonID is the counterparty the message was from (inbound) or to
	// (outbound). Zero when the address never matched a person — an unmatched
	// address is common, and privacy erasure actively nulls the link — so a
	// reader must treat this as "who, if we know".
	PersonID ids.UUID
}

// QuietFacts is what the sweep knows about a deal's silence: the last time each
// side spoke. Either may be absent on a deal that has only ever been one-way,
// or that predates activity capture.
type QuietFacts struct {
	LastInbound  *QuietSide
	LastOutbound *QuietSide
}

// The two directions, each paired with whose address identifies the
// counterparty on it. On an inbound message the counterparty WROTE it, so they
// are the sender; on an outbound one we wrote to them, so they are a recipient.
// Reading 'from' on an outbound message would name our own colleague and report
// the silence as theirs.
const (
	directionInbound  = "inbound"
	directionOutbound = "outbound"

	counterpartyRoleInbound  = "from"
	counterpartyRoleOutbound = "to"
)

// ReadQuietFacts reads the last inbound and last outbound message on the deal.
//
// It takes the caller's transaction rather than opening two, so both directions
// are read against one deal's state in one place.
//
// The two statements do NOT share a snapshot — the pool's transactions are
// READ COMMITTED, so a reply landing between them could be seen by the second
// arm and not the first. That is deliberate rather than overlooked: the worst
// case is one night's card naming the previous direction of a silence that has
// just ended, and the next sweep corrects it. Raising the isolation level to
// buy a snapshot would put a serialization failure in the path of a nightly
// hygiene pass, which is a worse trade than a card that is one day stale.
//
// Deliberately scoped by activity_link.deal_id rather than by the stakeholder
// walk EngagedStakeholders uses. The question here is about THIS deal's
// correspondence, and a stakeholder's unrelated mail about another deal at the
// same account is not evidence that this one is alive.
func ReadQuietFacts(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (QuietFacts, error) {
	// The object gate as well as the row scope below. They answer different
	// questions — MAY this caller read activities at all, and WHICH ones — and
	// the discover clause is only the second. Without this, an owner holding
	// deal:read but no activity grant would still have their correspondence
	// dates read and written into the card.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return QuietFacts{}, err
	}
	var facts QuietFacts
	for _, arm := range []struct {
		direction string
		role      string
		into      **QuietSide
	}{
		{directionInbound, counterpartyRoleInbound, &facts.LastInbound},
		{directionOutbound, counterpartyRoleOutbound, &facts.LastOutbound},
	} {
		side, found, err := readQuietSide(ctx, tx, dealID, arm.direction, arm.role)
		if err != nil {
			return QuietFacts{}, fmt.Errorf("last %s message on deal %s: %w", arm.direction, dealID, err)
		}
		if found {
			*arm.into = &side
		}
	}
	return facts, nil
}

// readQuietSide is one direction's arm, reporting whether the deal has a
// message that way at all. The participant lookup may find nobody: a message
// whose address never matched a person still tells the reader WHEN the side
// last spoke, and dropping it would report a deal as never-contacted because
// the address is unknown.
func readQuietSide(ctx context.Context, tx pgx.Tx, dealID ids.DealID, direction, role string) (QuietSide, bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealPos := arg(dealID)
	directionPos := arg(direction)
	rolePos := arg(role)
	// The caller runs this as the deal's OWNER, and this clause is what makes
	// that principal mean something: without it the read would return the same
	// rows whoever asked, and binding the owner would gate only the names while
	// the dates came from everywhere. Discover rather than content, because what
	// this reads is a marker — when a side last spoke — not a message body.
	scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return QuietSide{}, false, err
	}
	// ORDER BY occurred_at DESC LIMIT 1 rather than a FILTERed max(): the row
	// is what carries the counterparty, and idx_activity_direction serves this
	// shape directly.
	//
	// The discover clause carries the availability test (a retention-held row is
	// unavailable to every reader), so it is not spelled a second time here.
	//
	// It is a Sprintf ARGUMENT rather than concatenated into the format string:
	// a `%` ever appearing in it would otherwise be read as a verb and corrupt
	// the statement at runtime with nothing to catch it.
	//
	// The counterparty is named only when there is exactly ONE participant in
	// that role. A message to four people has no single person the silence
	// belongs to, and picking one — by id order or any other arbitrary rule —
	// would print a name the reader can check and find misleading. Group
	// correspondence therefore reports its dates with no name attached, which is
	// the true answer.
	//
	// The count is over EVERY participant in the role, not only the matched
	// ones. An address that never resolved to a person is still somebody on the
	// thread, so counting matches alone would read "one person plus three
	// unknown addresses" as a private exchange and name them for it.
	var side QuietSide
	var personID *ids.UUID
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT a.occurred_at, a.kind,
		       (SELECT sole.person_id FROM activity_participant sole
		         WHERE sole.activity_id = a.id AND sole.role = $%[3]d
		           AND sole.person_id IS NOT NULL
		           AND (SELECT count(*) FROM activity_participant every
		                 WHERE every.activity_id = a.id AND every.role = $%[3]d) = 1)
		FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $%[1]d
		WHERE a.archived_at IS NULL AND %[4]s AND a.direction = $%[2]d
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT 1`, dealPos, directionPos, rolePos, scope), args...).
		Scan(&side.At, &side.Kind, &personID)
	if errors.Is(err, pgx.ErrNoRows) {
		return QuietSide{}, false, nil
	}
	if err != nil {
		return QuietSide{}, false, err
	}
	if personID != nil {
		side.PersonID = *personID
	}
	return side, true, nil
}
