// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// applyOfferLinePatch moves the line when the patch names a place, then writes
// the rest of the patch, and answers the lines the move renumbered.
func applyOfferLinePatch(ctx context.Context, tx pgx.Tx, offerID ids.OfferID, lineID ids.UUID, place *int, sets []string, args []any) ([]placement, error) {
	var moves []placement
	if place != nil {
		var err error
		if moves, err = moveOfferLine(ctx, tx, offerID, lineID, *place); err != nil {
			return nil, err
		}
	}
	if len(sets) == 0 {
		return moves, nil
	}
	if _, err := tx.Exec(ctx,
		fmt.Sprintf(`UPDATE offer_line_item SET %s WHERE id = $1`, strings.Join(sets, ", ")),
		args...); err != nil {
		return nil, fmt.Errorf("apply line patch: %w", err)
	}
	return moves, nil
}

// moveOfferLine puts the line at place `to` in the offer's order, a place past
// the last line putting it last, and numbers the offer's lines 1..n. The caller
// holds the offer's lock, which every line write takes first.
func moveOfferLine(ctx context.Context, tx pgx.Tx, offerID ids.OfferID, lineID ids.UUID, to int) ([]placement, error) {
	lines, err := readOfferLineOrder(ctx, tx, offerID)
	if err != nil {
		return nil, err
	}
	moves, parks := lineMovePlan(lines, lineID, to)
	if len(moves) == 0 {
		return nil, nil
	}
	const place = `UPDATE offer_line_item SET position = v.position
		FROM unnest($1::uuid[], $2::int[]) AS v(id, position) WHERE offer_line_item.id = v.id`
	if err := parkThenPlace(ctx, tx, place, moves, parks, "offer lines"); err != nil {
		return nil, err
	}
	return moves, nil
}

// lineMovePlan answers the lines whose place changes and where each waits
// between the two statements. A line's position may not go below 1, so the
// parks are the smallest numbers above n that no line holds, and no rank is one.
func lineMovePlan(lines []ranked, lineID ids.UUID, to int) (moves []placement, parks []int) {
	at := slices.IndexFunc(lines, func(r ranked) bool { return r.id == lineID })
	if at < 0 {
		return nil, nil
	}
	moved := lines[at]
	order := slices.Delete(slices.Clone(lines), at, at+1)
	place := min(max(to, 1), len(lines)) - 1
	order = slices.Insert(order, place, moved)
	moves = placements(order)

	held := make(map[int]bool, len(lines))
	for _, r := range lines {
		held[r.position] = true
	}
	for park := len(lines) + 1; len(parks) < len(moves); park++ {
		if !held[park] {
			parks = append(parks, park)
		}
	}
	return moves, parks
}

// readOfferLineOrder answers the offer's lines in the order the offer reads
// them (readOfferLines), so a place counts the lines a reader sees above it.
func readOfferLineOrder(ctx context.Context, tx pgx.Tx, offerID ids.OfferID) ([]ranked, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, position FROM offer_line_item WHERE offer_id = $1 ORDER BY position, id`, offerID)
	if err != nil {
		return nil, fmt.Errorf("read the offer's line order: %w", err)
	}
	defer rows.Close()
	var lines []ranked
	for rows.Next() {
		var r ranked
		if err := rows.Scan(&r.id, &r.position); err != nil {
			return nil, fmt.Errorf("scan an offer line: %w", err)
		}
		lines = append(lines, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the offer's line order: %w", err)
	}
	return lines, nil
}
