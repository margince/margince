// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const mergeSocialTwin = `b.contact_id = @target AND b.platform = a.platform`

// MergeSocial pairs source social identity with any identity held by the survivor.
type MergeSocial struct {
	SourceID     ids.UUID
	Platform     string
	SourceHandle string
	TargetID     *ids.UUID
	TargetHandle *string
}

func previewMergeSocial(ctx context.Context, tx pgx.Tx, source, target ids.UUID) ([]MergeSocial, error) {
	if err := previewFieldsVisible(ctx, contactEntity, "social"); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.platform, a.handle, b.id, b.handle
 FROM contact_social a LEFT JOIN contact_social b ON `+mergeSocialTwin+`
 WHERE a.contact_id = @source ORDER BY a.platform, a.id LIMIT 501`, pgx.StrictNamedArgs{mergeSourceKey: source, mergeTargetKey: target})
	if err != nil {
		return nil, err
	}
	result, err := pgx.CollectRows(rows, pgx.RowToStructByPos[MergeSocial])
	if err != nil {
		return nil, err
	}
	if len(result) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return result, nil
}
