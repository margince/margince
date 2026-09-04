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
// Deals absent from the result are deals with no live seats at all, which is a
// different fact again — an untouched deal nobody has put a committee on has
// no coverage gap to report, it has no committee. The caller decides what that
// means; folding it in here would make an empty deal indistinguishable from an
// uncovered one.
//
// Every read carries the edge admission AND the person row scope, the same
// pair Stakeholders takes and for the same reason: reading a deal does not
// license learning who sits on it. A caller refused edges outright gets
// Withheld on every deal rather than an error, because a queue that failed
// whole because one lane is gated is worse than a queue that says less.
func ChampionCoverFor(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time,
) (map[ids.UUID]ChampionCover, error) {
	out := make(map[ids.UUID]ChampionCover, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	seats, withheld, err := championSeats(ctx, tx, dealIDs)
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

// dealPerson keys one seat: engagement is a fact about a person ON a deal, and
// a person can sit on two of them with a different answer on each.
type dealPerson struct {
	deal, person ids.UUID
}

// championSeats reads the champion-role seats on each deal, and reports which
// deals had a seat the reader could not see.
//
// The withheld count comes from the SAME statement rather than a second one
// over an unscoped view: a count read outside the row scope would itself
// disclose how many people sit on a deal the caller may not read. `visible` is
// the person scope, so `count(*) FILTER (WHERE NOT visible)` counts rows the
// join already admitted through the edge grant and the person scope refused —
// which is the boundary being reported, and no wider.
func championSeats(
	ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID,
) (map[ids.UUID][]ids.UUID, map[ids.UUID]bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealsPos := arg(dealIDs)
	rolePos := arg(dealrole.Champion)
	bound, err := edgeBound(ctx, "r", arg)
	if err != nil {
		return nil, nil, err
	}
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return nil, nil, err
	}
	visible := predicateAlways
	if scope != "" {
		visible = scope
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT r.deal_id,
		       array_remove(array_agg(r.person_id) FILTER (WHERE %[3]s AND r.role = $%[2]d), NULL),
		       count(*) FILTER (WHERE NOT (%[3]s)) > 0
		  FROM relationship r
		  JOIN person p ON p.id = r.person_id AND p.archived_at IS NULL
		 WHERE r.kind = 'deal_stakeholder' AND r.deal_id = ANY($%[1]d) AND r.archived_at IS NULL
		   AND (%[4]s)
		 GROUP BY r.deal_id`, dealsPos, rolePos, visible, bound), args...)
	if err != nil {
		return nil, nil, fmt.Errorf("deals: reading the champion seats on a set of deals: %w", err)
	}
	defer rows.Close()
	seats := make(map[ids.UUID][]ids.UUID, len(dealIDs))
	withheld := make(map[ids.UUID]bool, len(dealIDs))
	for rows.Next() {
		var deal ids.UUID
		var people []ids.UUID
		var anyWithheld bool
		if err := rows.Scan(&deal, &people, &anyWithheld); err != nil {
			return nil, nil, fmt.Errorf("deals: reading a deal's champion seats: %w", err)
		}
		seats[deal] = people
		withheld[deal] = anyWithheld
	}
	return seats, withheld, rows.Err()
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
