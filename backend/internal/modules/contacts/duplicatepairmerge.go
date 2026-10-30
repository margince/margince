// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PairWriters says which ends of a pair the caller may change.
type PairWriters struct{ Left, Right bool }

// MayMergePair is who may merge two records: write authority over both, or
// over either end of a pair the dedupe queue filed. Every surface that offers
// or performs a merge asks it, so the two owners of one duplicate can each
// resolve it.
func MayMergePair(writers PairWriters, filed bool) bool {
	return (writers.Left && writers.Right) || (filed && (writers.Left || writers.Right))
}

// admitDuplicatePairMerge answers the context a merge of source and target
// runs under. When only the filed pair admits the caller, the context admits
// both records, so every carrier the merge calls agrees with the door.
// Otherwise the ordinary write probes refuse as before.
//
// Both records must be live and visible to the caller. The caller holds the
// row locks, so the answer holds until commit.
func admitDuplicatePairMerge(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) (context.Context, error) {
	pair := []ids.UUID{source, target}
	writable, err := auth.PassingSubset(ctx, tx, kind, pair, auth.RuleWritable)
	if err != nil {
		return ctx, err
	}
	if writable[source] == writable[target] {
		return ctx, nil
	}
	live, err := auth.PassingSubset(ctx, tx, kind, pair, auth.RuleVisibleLive)
	if err != nil || !live[source] || !live[target] {
		return ctx, err
	}
	filed, err := duplicatePairFiled(ctx, tx, kind, source, target)
	if err != nil || !MayMergePair(PairWriters{Left: writable[source], Right: writable[target]}, filed) {
		return ctx, err
	}
	return auth.WithDuplicatePairAdmission(ctx, kind, source, target)
}

// duplicatePairFiled reports whether a live dedupe candidate names these two
// records and nobody has said they differ. A `merged` row counts
// because the queue and a case merge mark their pair before the merge runs, in
// the same transaction. The share lock makes a concurrent dismissal wait.
func duplicatePairFiled(ctx context.Context, tx pgx.Tx, kind string, a, b ids.UUID) (bool, error) {
	side, ok := dedupePairSideFor(kind)
	if !ok {
		return false, fmt.Errorf("contacts: %q has no duplicate pairs", kind)
	}
	var id ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM dedupe_candidate
		 WHERE entity_type = @entity AND archived_at IS NULL AND disposition IN ('open', 'merged')
		   AND %[1]s = LEAST(@a::uuid, @b::uuid) AND %[2]s = GREATEST(@a::uuid, @b::uuid)
		   FOR SHARE`, side.left, side.right),
		pgx.NamedArgs{"entity": side.entity, "a": a, "b": b}).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading whether the pair is a filed duplicate: %w", err)
	}
	return true, nil
}
