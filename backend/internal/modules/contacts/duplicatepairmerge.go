// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MergeWriters says which records of one merge the caller may change: the
// source merged away and the target that survives.
type MergeWriters struct{ Source, Target bool }

// MayMergePair is who may merge source into target: write authority over
// both, or over the source alone when the dedupe queue filed the pair. The
// one-sided merge only gives the caller's own record away. The colleague's
// record survives with its owner, so editing one's own record to look like a
// colleague's never archives theirs.
func MayMergePair(writers MergeWriters, filed bool) bool {
	return (writers.Source && writers.Target) || (filed && writers.Source)
}

// PairWriters says which ends of a filed pair the caller may change.
type PairWriters struct{ Left, Right bool }

// MergeOffer is what a reader of a filed pair may merge. Keep names the one
// record that must survive when only one direction is admitted.
type MergeOffer struct {
	May  bool
	Keep *ids.UUID
}

// OfferMerge asks MayMergePair of both directions of a pair.
func OfferMerge(left, right ids.UUID, writers PairWriters, filed bool) MergeOffer {
	keepLeft := MayMergePair(MergeWriters{Source: writers.Right, Target: writers.Left}, filed)
	keepRight := MayMergePair(MergeWriters{Source: writers.Left, Target: writers.Right}, filed)
	switch {
	case keepLeft && keepRight:
		return MergeOffer{May: true}
	case keepLeft:
		return MergeOffer{May: true, Keep: &left}
	case keepRight:
		return MergeOffer{May: true, Keep: &right}
	}
	return MergeOffer{}
}

// errMergeIntoYours refuses a one-sided merge that would archive the record
// the caller may not change.
var errMergeIntoYours = &mergeDirectionError{}

type mergeDirectionError struct{}

func (*mergeDirectionError) Error() string {
	return "contacts: you may change only one of these records, so it is the one merged away"
}

func (*mergeDirectionError) MessageFault() (code, message string) {
	return "merge_direction", "Merge your record into your colleague's, which keeps theirs and its owner, or ask its owner to merge."
}

func (*mergeDirectionError) Unwrap() error { return apperrors.ErrPermissionDenied }

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
	writers := MergeWriters{Source: writable[source], Target: writable[target]}
	if writers.Source == writers.Target {
		return ctx, nil
	}
	live, err := auth.PassingSubset(ctx, tx, kind, pair, auth.RuleVisibleLive)
	if err != nil || !live[source] || !live[target] {
		return ctx, err
	}
	filed, err := duplicatePairFiled(ctx, tx, kind, source, target)
	switch {
	case err != nil || !filed:
		return ctx, err
	case !MayMergePair(writers, filed):
		return ctx, errMergeIntoYours
	}
	return auth.WithDuplicatePairAdmission(ctx, kind, source, target)
}

// duplicatePairFiled reports whether a live dedupe candidate names these two
// records and nobody has said they differ. A `merged` row counts only when
// this transaction marked it, as the queue and a case merge do before the
// merge runs. The share lock makes a concurrent dismissal wait.
func duplicatePairFiled(ctx context.Context, tx pgx.Tx, kind string, a, b ids.UUID) (bool, error) {
	side, ok := dedupePairSideFor(kind)
	if !ok {
		return false, fmt.Errorf("contacts: %q has no duplicate pairs", kind)
	}
	var id ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT id FROM dedupe_candidate
		 WHERE entity_type = @entity AND archived_at IS NULL
		   AND (disposition = 'open' OR (disposition = 'merged' AND disposed_at = now()))
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
