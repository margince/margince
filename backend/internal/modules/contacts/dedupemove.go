// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// moveDedupeCandidates re-homes the pairs naming a merged-away contact or
// company onto its survivor, so a third record's open pair stays open. Rows are
// re-sorted because `dedupe_candidate_ordered` wants left < right. The unique
// pair key spans every disposition, so a pair the survivor holds already is
// dropped, the one without a human verdict first. The pair the merge settles
// stays as its record. Columns come from dedupePairSides.
func moveDedupeCandidates(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	side, ok := dedupePairSideFor(entityType)
	if !ok {
		return fmt.Errorf("contacts: %q has no duplicate pairs to move", entityType)
	}
	// otherEnd is the end the retired record does not occupy, read off the row
	// as it was before the statement, so both SET expressions agree.
	otherEnd := func(row string) string {
		return fmt.Sprintf("CASE WHEN %[1]s.%[2]s = @source THEN %[1]s.%[3]s ELSE %[1]s.%[2]s END",
			row, side.left, side.right)
	}
	sameNormalizedPair := fmt.Sprintf(`LEAST(b.%[1]s, b.%[2]s) = LEAST(@target::uuid, %[3]s)
	        AND GREATEST(b.%[1]s, b.%[2]s) = GREATEST(@target::uuid, %[3]s)`, side.left, side.right, otherEnd("a"))
	namesSource := func(row string) string {
		return fmt.Sprintf("(%[1]s.%[2]s = @source OR %[1]s.%[3]s = @source)", row, side.left, side.right)
	}
	if err := settleOpenPairBetween(ctx, tx, side, sourceID, targetID); err != nil {
		return err
	}
	steps := []struct{ what, stmt string }{
		// A disposition is a human answer and outlives an open row; between two
		// rows that both carry one, the survivor's own stands.
		{"drop the duplicated pairs", `
		DELETE FROM dedupe_candidate a
		 WHERE a.entity_type = @entity AND ` + namesSource("a") + `
		   AND EXISTS (
		     SELECT 1 FROM dedupe_candidate b
		      WHERE b.entity_type = @entity AND b.id <> a.id AND ` + sameNormalizedPair + `
		        AND (b.disposition <> 'open' OR a.disposition = 'open'))`},
		{"drop the survivor's undecided duplicates", `
		DELETE FROM dedupe_candidate b
		 WHERE b.entity_type = @entity AND b.disposition = 'open'
		   AND EXISTS (
		     SELECT 1 FROM dedupe_candidate a
		      WHERE a.entity_type = @entity AND a.id <> b.id AND a.disposition <> 'open'
		        AND ` + namesSource("a") + ` AND ` + sameNormalizedPair + `)`},
		// The pair BETWEEN the two merging records is left as it is at every
		// disposition: it records the merge that retired one of them.
		{"re-normalize the remaining pairs", fmt.Sprintf(`
		UPDATE dedupe_candidate p SET
		  %[1]s = LEAST(@target::uuid, %[3]s),
		  %[2]s = GREATEST(@target::uuid, %[3]s)
		 WHERE p.entity_type = @entity AND %[4]s AND %[3]s <> @target`,
			side.left, side.right, otherEnd("p"), namesSource("p"))},
	}
	args := pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, "entity": entityType}
	for _, step := range steps {
		if _, err := tx.Exec(ctx, step.stmt, args); err != nil {
			return fmt.Errorf("contacts: %s: %w", step.what, err)
		}
	}
	return nil
}

// settleOpenPairBetween marks the open pair of the two merging records as
// merged, by the user behind this call. The queue and a case merge mark their
// pairs first. So only a merge from a record page finds one open, and without
// this it left no row for the merged list to show.
func settleOpenPairBetween(ctx context.Context, tx pgx.Tx, side dedupePairSide, sourceID, targetID ids.UUID) error {
	var pairID ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM dedupe_candidate
		 WHERE entity_type = @entity AND disposition = 'open'
		   AND ((%[1]s = @source AND %[2]s = @target) OR (%[1]s = @target AND %[2]s = @source))`,
		side.left, side.right),
		pgx.NamedArgs{mergeSourceKey: sourceID, mergeTargetKey: targetID, "entity": side.entity}).Scan(&pairID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: reading the open pair of the merging records: %w", err)
	}
	actor, _ := principal.Actor(ctx)
	return setDedupeDispositionTx(ctx, tx, pairID, dispositionMerged, actor.UserID)
}
