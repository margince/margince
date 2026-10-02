// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two things a capture settles BEFORE it writes anything: the merge lock it
// must hold first, and whether an invitation already filed this meeting.

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// takeMergeLockFirst takes the thread-merge lock BEFORE this capture writes a
// row, when the record is a reply that could reach a stored message.
//
// The ordering is the whole point. mergeLinkedThreads takes the same lock and
// then updates every row of the threads it merges; a concurrent capture that has
// already written — and so locked — one of those rows and then waits for the
// merge lock closes a cycle, and Postgres breaks it by aborting one of them. The
// aborted capture is retried and loses nothing, so a busy shared conversation
// captured by two mailboxes pays for it in repeated retries rather than in data.
//
// Taking the coarse lock before any fine one removes the cycle: a transaction
// that will ever hold this lock holds it before it owns a row anybody else wants.
//
// The probe is what keeps the lock off the common path. It asks the neighbour
// question this capture would ask later, minus the exclusion of its own
// not-yet-written row, so it can only answer with MORE rows than the real join
// finds, never fewer — a capture that merges never slips past it. A non-email
// record and a message nothing stored links skip the lock entirely.
//
// It is NOT gated on the record carrying References of its own, and that is the
// whole reason it asks the database rather than reading the header. MailNeighbours
// matches a stored message that references THIS one as well as the ones this one
// references, so a backfill capturing a parent after its reply has a neighbour
// while its own header names nobody. Gating on ReplyTo skipped the lock exactly
// there, and left the deadlock ordering in place for the one capture order that
// produces it.
func (s *Sink) takeMergeLockFirst(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		return nil
	}
	linked, err := s.threadJoin.Neighbours(ctx, tx, ids.ActivityID{}, rec.NaturalKey.SourceID, rec.ReplyTo)
	if err != nil || len(linked) == 0 {
		return err
	}
	if _, err := tx.Exec(ctx, mergeLockStatement); err != nil {
		return fmt.Errorf("capture: taking the thread merge lock before the row: %w", err)
	}
	return nil
}

// invitationAlreadyFiled answers the row a meeting invitation already filed, so a
// capture of the same meeting takes it over rather than minting a second row.
func (s *Sink) invitationAlreadyFiled(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (datasource.EntityRef, bool, error) {
	if fields.Kind != meetingKind || s.resolveInvitation == nil {
		return datasource.EntityRef{}, false, nil
	}
	id, found, err := s.resolveInvitation(ctx, tx, rec.NaturalKey, rec.Raw)
	if err != nil || !found {
		return datasource.EntityRef{}, false, err
	}
	return datasource.EntityRef{Type: datasource.EntityActivity, ID: id.UUID}, true, nil
}
