// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The one lock every writer of communication_suppression takes.
//
// Three writers touch this table for a given subject: Suppress records a stop,
// Lift revokes one, and CarryStopsTx copies a retiring subject's stops onto the
// record that survives them. Each reads the subject's live rows and decides
// what to write from what it saw, so two of them running at once can lose a
// stop outright — the failure the carry exists to prevent, reintroduced by the
// carry's own concurrency.
//
// The losing interleaving that motivated this file:
//
//	Suppress(A, marketing_objection)   CarryStops(A -> B)
//	   auth, EnsureWritable                 reads A's live rows: none
//	   INSERT onto A                        writes nothing onto B
//	   COMMIT                               COMMIT
//
// A holds the objection, B holds nothing, and B is the record every send now
// evaluates. Neither transaction did anything wrong; they simply never queued.
// Suppress took no lock at all before this, so locking only the survivor inside
// the carry could not have closed it.
//
// TRANSACTION-SCOPED, so the lock is released by COMMIT or ROLLBACK and no
// caller can leak one. The key is hashtextextended over the subject's uuid text,
// which is what lift.go took before this file existed — the spelling is kept so
// a transaction taken by an older code path and one taken here collide as they
// should.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// lockSubjectSuppressions serialises every writer of one subject's stops.
func lockSubjectSuppressions(ctx context.Context, tx pgx.Tx, subject ids.UUID) error {
	if subject.IsZero() {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, subject.String()); err != nil {
		return fmt.Errorf("consent: serialising stop writes for this subject: %w", err)
	}
	return nil
}

// lockBothSidesOfACarry takes the lock on the retiring subject AND the
// survivor, because the carry reads one and writes the other. Locking only the
// survivor left the read side open, which is the hole described above.
//
// ORDERED BY UUID, not by which side is retiring. Two carries naming the same
// pair in opposite directions — a merge and a concurrent merge back — would
// otherwise take the two locks in opposite orders and deadlock. Sorting makes
// the acquisition order a property of the pair rather than of the caller.
func lockBothSidesOfACarry(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	return lockSubjectsInOrder(ctx, tx, subjectKey(from), subjectKey(to))
}

// lockSubjectsInOrder takes each subject's lock once, in uuid order.
//
// SORTED, not in the order the caller happened to name them. Two carries
// naming the same pair in opposite directions — a merge and a concurrent merge
// back — would otherwise take the two locks in opposite orders and deadlock.
// Sorting makes the acquisition order a property of the pair rather than of
// the caller.
func lockSubjectsInOrder(ctx context.Context, tx pgx.Tx, subjects ...ids.UUID) error {
	keys := make([]string, 0, len(subjects))
	for _, s := range subjects {
		if !s.IsZero() {
			keys = append(keys, s.String())
		}
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	for _, k := range keys {
		if _, err := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, k); err != nil {
			return fmt.Errorf("consent: serialising stop writes for this subject: %w", err)
		}
	}
	return nil
}

// subjectKey is the one id a StopSubject locks under. Exactly one of the two is
// set, so this is a selection and never a choice.
func subjectKey(s commsauthz.StopSubject) ids.UUID {
	if !s.PersonID.IsZero() {
		return s.PersonID.UUID
	}
	return s.LeadID.UUID
}
