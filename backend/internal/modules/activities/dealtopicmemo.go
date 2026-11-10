// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The deal topic check's memo: which candidate deals and prompt it last
// answered for a message. The hourly passes read it, and do not ask the model
// the same question twice. It is bookkeeping about the passes, not a judgement
// of the message. So it writes no audit row and no event, as the label-decline
// stamp does.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StampDealTopicCheckedTx records that the topic check answered digest for the
// message its caller holds locked, and answers whether it did. A writer
// without activity.update records nothing. A held or archived row is left
// alone.
func StampDealTopicCheckedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, digest string) (bool, error) {
	if digest == "" {
		return false, errors.New("activities: a deal topic memo needs the digest it answered")
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE activity SET deal_topic_digest = $2, deal_topic_checked_at = now()
		 WHERE id = $1 AND restricted_at IS NULL AND archived_at IS NULL`, id, digest)
	if err != nil {
		return false, fmt.Errorf("activities: recording the deal topic check: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
