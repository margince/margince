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

// takeMergeLockFirst takes the thread-merge lock before this capture writes a
// row, when the record is a reply that could reach a stored message.
//
// mergeLinkedThreads takes the same lock and then updates every row of the
// threads it merges. A concurrent capture may already have written one of those
// rows, and so locked it, and then wait for the merge lock. That closes a cycle,
// and Postgres aborts one of the two. The aborted capture is retried and loses
// nothing, but a busy shared conversation pays in repeated retries.
//
// Taking the coarse lock before any fine one removes the cycle. A transaction
// that will hold this lock holds it before it owns a row anybody else wants.
//
// The probe keeps the lock off the common path. It asks the neighbour question
// this capture would ask later, minus the exclusion of its own unwritten row.
// So it can only answer with more rows than the real join finds, never fewer. A
// non-email record and a message nothing stored links skip the lock.
//
// The probe asks the database instead of reading the record's References.
// MailNeighbours matches a stored message that references this one as well as
// the ones this one references. A backfill that captures a parent after its
// reply has a neighbour while its own header names nobody. That capture order is
// the one that produces the deadlock.
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
