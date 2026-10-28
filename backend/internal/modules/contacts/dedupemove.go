// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// moveDedupeCandidates re-homes the duplicate pairs that name a merged-away
// contact or company onto its survivor.
//
// Three constraints shape it:
//
//   - dedupe_candidate_ordered requires left < right, so the rows are
//     re-normalized rather than repointed in place.
//   - uq_dedupe_candidate_pair spans EVERY disposition, so a merged or
//     dismissed pair collides exactly like an open one.
//   - the pair a merge is settling is marked 'merged' in this same transaction
//     before the merge runs, and that row must survive as the record of it.
//
// A pair naming both endpoints would become a self-pair and is dropped while
// open; a pair the survivor already holds is dropped as the duplicate it now
// is, the one carrying no human judgement going first. Everything else
// re-normalizes onto the survivor, so a third record's open pair with the
// retired one stays open against the survivor.
//
// The columns come from dedupePairSides, compile-time literals.
func moveDedupeCandidates(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	side, ok := dedupePairSideFor(entityType)
	if !ok {
		return fmt.Errorf("contacts: %q has no duplicate pairs to move", entityType)
	}
	// otherEnd is the column the retired record does NOT occupy, read off the
	// row as it was before the statement, so both SET expressions agree.
	otherEnd := func(row string) string {
		return fmt.Sprintf("CASE WHEN %[1]s.%[2]s = @source THEN %[1]s.%[3]s ELSE %[1]s.%[2]s END",
			row, side.left, side.right)
	}
	sameNormalizedPair := fmt.Sprintf(`LEAST(b.%[1]s, b.%[2]s) = LEAST(@target::uuid, %[3]s)
	        AND GREATEST(b.%[1]s, b.%[2]s) = GREATEST(@target::uuid, %[3]s)`, side.left, side.right, otherEnd("a"))
	namesSource := func(row string) string {
		return fmt.Sprintf("(%[1]s.%[2]s = @source OR %[1]s.%[3]s = @source)", row, side.left, side.right)
	}
	steps := []struct{ what, stmt string }{
		{"drop the open self-pair", fmt.Sprintf(`
		DELETE FROM dedupe_candidate p
		 WHERE p.entity_type = @entity AND p.disposition = 'open'
		   AND ((p.%[1]s = @source AND p.%[2]s = @target) OR (p.%[1]s = @target AND p.%[2]s = @source))`,
			side.left, side.right)},
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
