// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const mergeLiveChannelIdentity = `contact_id = @source AND archived_at IS NULL`

// MergeChannelIdentity keeps channel identity provenance visible when contacts merge.
type MergeChannelIdentity struct {
	ID       ids.UUID
	Provider string
	Account  string
	Username *string
	Blocked  bool
	Version  int64
}

func previewMergeChannelIdentities(ctx context.Context, tx pgx.Tx, source ids.UUID) ([]MergeChannelIdentity, error) {
	if err := previewFieldsVisible(ctx, contactEntity, "reachability"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id, provider, channel_user_id, username, blocked_at IS NOT NULL, version
 FROM contact_channel_identity WHERE `+mergeLiveChannelIdentity+` ORDER BY provider, id LIMIT 501`, pgx.StrictNamedArgs{mergeSourceKey: source})
	if err != nil {
		return nil, err
	}
	result, err := pgx.CollectRows(rows, pgx.RowToStructByPos[MergeChannelIdentity])
	if err != nil {
		return nil, err
	}
	if len(result) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return result, nil
}
