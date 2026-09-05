// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// "Is anybody inside the account carrying this deal?", asked of MANY deals at
// once.
//
// The per-deal answer already exists: Stakeholders lists every seat with its
// role and engagement, and the coverage rules fold it. That shape is right for
// one deal on a page and wrong for a queue, which weighs up to fifty drifting
// deals in a morning assembly — two statements each is a hundred round trips
// on the hot path for one boolean per row.
//
// So this asks the narrow question over the whole set in two statements,
// through the SAME engagement definition EngagedStakeholders spells. It is not
// a second coverage rule: it answers strictly less, and ChampionCover.Covered
// is the one fact it reports.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ChampionCover is what one deal's committee says about its champion seat.
//
// TWO fields rather than one boolean, because "no engaged champion" and "we
// could not see the whole committee" are different sentences to a rep and the
// second must never be told as the first. A champion the reader may not read
// is still a champion: the seat is absent from every row-scoped read, so a
// bare Covered=false over a partial committee is a claim the data does not
// support — the deal HAS somebody arguing for it and the queue would say
// nobody does.
type ChampionCover struct {
	// Covered is true when a seat with the champion role is engaged.
	Covered bool
	// Withheld is true when this reader could not read every live seat, which
	// makes Covered unsafe to report as a finding. Callers state the gap or
	// say nothing; they never round it down to "no champion".
	Withheld bool
}

// ChampionCoverFor answers the champion question for each of the given deals.
//
// A deal is absent from the result when NEITHER statement returned it: no live
// seat was readable and none was refused, which is an untouched deal nobody has
// put a committee on. It has no coverage gap to report because it has no
// committee. The caller decides what that means; folding it in here would make
// an empty deal indistinguishable from an uncovered one.
//
// The visible read carries the edge admission — reading a deal does not license
// learning who sits on it, the same reason Stakeholders takes it. The withheld
// probe carries that admission's COMPLEMENT and no object gate of its own,
// because this function resolved the gate before reaching it.
//
// A caller refused edges outright gets an error rather than a degraded answer;
// the lane above decides whether to show a partial at-risk queue or none.
func ChampionCoverFor(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time,
) (map[ids.UUID]ChampionCover, error) {
	out := make(map[ids.UUID]ChampionCover, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	seats, err := championSeats(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	withheld, err := withheldSeats(ctx, tx, dealIDs)
	if err != nil {
		return nil, err
	}
	for deal, isWithheld := range withheld {
		out[deal] = ChampionCover{Withheld: isWithheld}
	}
	if len(seats) == 0 {
		return out, nil
	}
	engaged, err := engagedAmong(ctx, tx, dealIDs, now)
	if err != nil {
		return nil, err
	}
	for deal, people := range seats {
		cover := out[deal]
		for _, person := range people {
			if engaged[dealPerson{deal: deal, person: person}] {
				cover.Covered = true
				break
			}
		}
		out[deal] = cover
	}
	return out, nil
}

// livePersonSeat is what both statements in this file mean by a seat: an edge
// whose person is still live. championSeats needs the row, so it joins; the
// withheld probe needs only the fact, so it asks EXISTS on the same condition.
// Two statements that disagree about what a seat is answer one question two
// ways: a deal would carry a withheld seat the visible read does not count, and
// report a committee it does not have.
//
// Held by TestOneSpellingOfALiveChampionSeat (championseat_test.go), which
// fails when either statement writes the condition out instead of naming this.
const livePersonSeat = "p.id = r.person_id AND p.archived_at IS NULL"

// dealPerson keys one seat: engagement is a fact about a person ON a deal, and
// a person can sit on two of them with a different answer on each.
type dealPerson struct {
	deal, person ids.UUID
}

// championSeats lists the champion-role seats this reader may see on each deal.
//
// The edge bound is a conjunction over every endpoint an edge carries, so a
// seat this reader may not see is already gone from these rows — refused by
// whichever arm refuses it, which is not always the person one. That is why the
// withheld question cannot be answered here, and why withheldSeats asks it in a
// statement of its own, as the complement of this same conjunction.
func championSeats(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID,
) (map[ids.UUID][]ids.UUID, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealsPos := arg(dealIDs)
	rolePos := arg(dealrole.Champion)
	bound, err := edgeBound(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.deal_id,
		       array_remove(array_agg(r.person_id) FILTER (WHERE r.role = $%[2]d), NULL)
		  FROM relationship r
		  JOIN person p ON %[4]s
		 WHERE r.kind = 'deal_stakeholder' AND r.deal_id = ANY($%[1]d) AND r.archived_at IS NULL
		   AND (%[3]s)
		 GROUP BY r.deal_id`, dealsPos, rolePos, bound, livePersonSeat), args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading the champion seats on a set of deals: %w", err)
	}
	defer rows.Close()
	seats := make(map[ids.UUID][]ids.UUID, len(dealIDs))
	for rows.Next() {
		var deal ids.UUID
		var people []ids.UUID
		if err := rows.Scan(&deal, &people); err != nil {
			return nil, fmt.Errorf("deals: reading a deal's champion seats: %w", err)
		}
		seats[deal] = people
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deals: reading the champion seats on a set of deals: %w", err)
	}
	return seats, nil
}

// withheldSeats answers, per deal, whether a seat on it is one this reader may
// not read.
//
// THE COMPLEMENT OF THE VISIBLE READ, not a reading of one of its arms. An edge
// is admitted by a conjunction over EVERY endpoint it carries — person_id,
// counterparty_person_id, organization_id, counterparty_org_id, deal_id,
// project_id (auth.RelationshipEndpointScope) — so "a seat this reader may not
// read" is exactly `NOT (that conjunction)`. A single arm answers a narrower
// question that looks the same until a seat is refused by one of the other five,
// and then reports a committee as fully readable while a champion sits in it.
//
// A seat refused through `counterparty_org_id` is the reachable case rather than
// the theoretical one: `rel_stakeholder_shape` pins organization_id, project_id
// and counterparty_person_id to NULL on a deal_stakeholder and says nothing
// about counterparty_org_id, and CreateRelationshipInput accepts it.
//
// Negating the whole clause is also what keeps this true as the edge grows: a
// seventh endpoint column reaches this statement the day it is added.
//
// It carries NO deal predicate, and that is a decision rather than an omission.
// `deal` is an identity table (auth/tableclass.go), so ScopeClauseFor answers ""
// for every SEATED human — the only principal that reaches this lane — and a
// clause here would render `true`, a guard shaped like one that narrows nothing.
// (A buyer is answered differently: readsEveryRow refuses one before it consults
// the identity set, so a future Deal Room caller would need its own bound rather
// than inheriting this reasoning.) What bounds the rows is the workspace
// transaction every caller opens, and the ids are the caller's own — the sole
// caller today is compose/attentionchampioncover.go, which passes the drifting
// deals its own sweep already selected.
//
// ONE BIT per deal, and a boolean rather than a count: how many seats a reader
// may not see is a fact about people they may not read, and a number would leak
// the size of a committee that a boolean does not.
//
// A deal absent from the result had no unreadable seat. A deal absent from BOTH
// this and the seat list has no committee at all, which the caller tells apart
// by asking the seat map — the two absences mean different things and neither
// is a claim about a champion.
func withheldSeats(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealsPos := arg(dealIDs)
	admitted, err := auth.RelationshipEndpointScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	// An empty clause cannot be negated: `NOT ()` is a syntax error, not a
	// permissive read. RelationshipEndpointScope answers "" only for the system
	// principal — UnboundedFor refuses every other actor at `person` and
	// `organization`, which carry capture privacy — and the system principal is
	// refused no row, so nothing is withheld from it.
	if admitted == "" {
		return map[ids.UUID]bool{}, nil
	}
	// livePersonSeat is what championSeats counts as a seat, asked here so the
	// two statements share one definition rather than two spellings that agree
	// by inspection. An edge admitted by neither — archived person, refused
	// endpoint — would otherwise be a withheld seat to this statement and no
	// seat at all to that one, and the deal would report a committee it does
	// not have.
	//
	// The endpoint conjunction alone, WITHOUT the object gate EdgeReadScope
	// takes ahead of it: championSeats resolved that gate before this runs and a
	// refused caller returned there, so asking again is a second Require on one
	// request.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT r.deal_id
		  FROM relationship r
		 WHERE r.kind = 'deal_stakeholder' AND r.deal_id = ANY($%[1]d) AND r.archived_at IS NULL
		   AND EXISTS (SELECT 1 FROM person p WHERE %[3]s)
		   AND NOT (%[2]s)`, dealsPos, admitted, livePersonSeat), args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading which deals carry a seat this reader may not see: %w", err)
	}
	defer rows.Close()
	withheld := make(map[ids.UUID]bool, len(dealIDs))
	for rows.Next() {
		var deal ids.UUID
		if err := rows.Scan(&deal); err != nil {
			return nil, fmt.Errorf("deals: reading a deal with a withheld seat: %w", err)
		}
		withheld[deal] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deals: reading which deals carry a seat this reader may not see: %w", err)
	}
	return withheld, nil
}

// engagedAmong answers which (deal, person) pairs had a two-way exchange in
// the window.
//
// The engagement test is EngagedStakeholders' — both directions required
// inside the same window, over the same qualifying kinds — asked of a set
// rather than one deal. Two spellings of "engaged" is how one screen calls a
// deal single-threaded while another calls it covered, which is the
// disagreement the single-definition rule in this package's header exists to
// stop; the shared pieces are healthEngagementWindowDays and
// healthActivityKinds, so a change to either moves both readers at once.
func engagedAmong(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time,
) (map[dealPerson]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealsPos := arg(dealIDs)
	windowPos := arg(now.AddDate(0, 0, -healthEngagementWindowDays))
	bound, err := edgeBound(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	// The kind list is an ARGUMENT to Sprintf rather than concatenated into
	// its format string, for the reason EngagedStakeholders gives: a `%` in a
	// kind would be read as a verb and corrupt the statement at runtime.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT r.deal_id, r.person_id FROM relationship r
		WHERE r.kind = 'deal_stakeholder' AND r.deal_id = ANY($%[1]d) AND r.archived_at IS NULL
		  AND (%[3]s)
		  AND EXISTS (
			SELECT 1 FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.person_id = r.person_id
			WHERE a.kind IN %[4]s AND a.archived_at IS NULL
			  AND a.occurred_at >= $%[2]d AND a.direction = 'inbound')
		  AND EXISTS (
			SELECT 1 FROM activity a
			JOIN activity_link l ON l.activity_id = a.id AND l.person_id = r.person_id
			WHERE a.kind IN %[4]s AND a.archived_at IS NULL
			  AND a.occurred_at >= $%[2]d AND a.direction = 'outbound')`,
		dealsPos, windowPos, bound, healthActivityKinds), args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading engagement across a set of deals: %w", err)
	}
	defer rows.Close()
	engaged := make(map[dealPerson]bool)
	for rows.Next() {
		var pair dealPerson
		if err := rows.Scan(&pair.deal, &pair.person); err != nil {
			return nil, fmt.Errorf("deals: reading one deal's engaged seat: %w", err)
		}
		engaged[pair] = true
	}
	return engaged, rows.Err()
}
