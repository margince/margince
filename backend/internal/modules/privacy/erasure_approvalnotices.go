// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/noticekeys"
)

// Lock the source first so notification fan-out cannot restore a copy after erasure.
// The predicate comes from the same erasure scope that clears the source proposal.
func eraseApprovalNoticesMatching(ctx context.Context, tx pgx.Tx, reason, cause, predicate string, args ...any) error {
	rows, err := tx.Query(ctx, `SELECT id FROM approval WHERE `+predicate+` ORDER BY created_at, id FOR UPDATE`, args...)
	if err != nil {
		return fmt.Errorf("privacy: reading approval copies to erase: %w", err)
	}
	approvals, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(approvals))
	for _, id := range approvals {
		keys = append(keys, noticekeys.Approval(id))
	}
	if len(keys) == 0 {
		return nil
	}
	rows, err = tx.Query(ctx, `UPDATE notice
 SET subject = 'Proposal removed', body = '', origin = NULL, email_error = NULL, retracted_at = coalesce(retracted_at, now())
 WHERE dedupe_key = ANY(@keys) RETURNING id`, pgx.StrictNamedArgs{"keys": keys})
	if err != nil {
		return fmt.Errorf("privacy: erasing copied approval notices: %w", err)
	}
	cleared, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	return tombstoneCollateralScrubs(ctx, tx, "notice", cleared, reason, cause)
}
