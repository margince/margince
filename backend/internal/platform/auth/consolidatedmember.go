// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ConsolidatedMemberTx resolves forward for a writer holding an older identity.
// The share lock serializes it with retirement; readers use the reverse family.
func ConsolidatedMemberTx(ctx context.Context, tx pgx.Tx, member ids.UUID) (ids.UUID, bool, error) {
	if _, err := rbacActor(ctx); err != nil {
		return ids.UUID{}, false, err
	}
	seen := map[ids.UUID]bool{}
	for !member.IsZero() {
		if seen[member] {
			return ids.UUID{}, false, apperrors.ErrConflict
		}
		seen[member] = true
		var removed *time.Time
		var merged *ids.UUID
		err := tx.QueryRow(ctx, `SELECT removed_at, merged_into_user_id FROM app_user WHERE id = @member AND archived_at IS NULL FOR KEY SHARE`, pgx.StrictNamedArgs{"member": member}).Scan(&removed, &merged)
		if errors.Is(err, pgx.ErrNoRows) {
			return ids.UUID{}, false, nil
		}
		if err != nil {
			return ids.UUID{}, false, err
		}
		if removed == nil {
			return member, true, nil
		}
		if merged == nil {
			return ids.UUID{}, false, nil
		}
		member = *merged
	}
	return ids.UUID{}, false, nil
}
