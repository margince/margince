// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two things a capture settles BEFORE it writes anything: the merge lock it
// must hold first, and whether an invitation already filed this meeting.

package capture

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// takeMergeLockFirst fences identities and components before capture owns rows.
// Re-reading catches a canonical key that moved while the first locks waited.
func (s *Sink) takeMergeLockFirst(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		return nil
	}
	keys, err := s.captureThreadLocks(ctx, tx, rec)
	if err != nil {
		return err
	}
	if err := storekit.WaitThreadKeysTx(ctx, tx, keys); err != nil {
		return err
	}
	current, err := s.captureThreadLocks(ctx, tx, rec)
	if err != nil {
		return err
	}
	if slices.Equal(keys, current) {
		return nil
	}
	if err := storekit.LockThreadKeysTx(ctx, tx, current...); err != nil {
		return err
	}
	stable, err := s.captureThreadLocks(ctx, tx, rec)
	if err != nil {
		return err
	}
	for _, key := range stable {
		if !slices.Contains(keys, key) && !slices.Contains(current, key) {
			return storekit.RetryThreadTransaction(ctx, tx)
		}
	}
	return nil
}

func (s *Sink) captureThreadLocks(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) ([]string, error) {
	keys := append(slices.Clone(rec.ReplyTo), rec.NaturalKey.SourceID)
	linked, err := s.threadJoin.Neighbours(ctx, tx, ids.ActivityID{}, rec.NaturalKey.SourceID, keys)
	if err != nil {
		return nil, err
	}
	keys = append(keys, rec.ThreadKey)
	if len(linked) > 0 {
		rows, err := tx.Query(ctx, `SELECT DISTINCT thread_key FROM activity WHERE id = ANY(@linked::uuid[]) AND thread_key IS NOT NULL`, pgx.NamedArgs{"linked": linked})
		if err != nil {
			return nil, err
		}
		current, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return nil, err
		}
		keys = append(keys, current...)
	}
	slices.Sort(keys)
	return slices.Compact(keys), nil
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
