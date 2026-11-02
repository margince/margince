// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two things a capture settles BEFORE it writes anything: the merge lock it
// must hold first, and whether an invitation already filed this meeting.

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// takeMergeLockFirst serializes mail births with bounded thread repair before row locks.
func (s *Sink) takeMergeLockFirst(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) error {
	if !s.threadJoin.complete() || rec.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		return nil
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
