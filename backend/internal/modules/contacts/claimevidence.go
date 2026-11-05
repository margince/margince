// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func claimContentClause(ctx context.Context, arg func(any) int) (string, error) {
	source, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", err
	}
	renewal, err := auth.CommitmentEvidenceClause(ctx, "c.source_activity_id", "c.evidence_fingerprint", arg)
	if err != nil {
		return "", err
	}
	return source + " AND " + renewal, nil
}

func ensureClaimContentVisible(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	content, err := claimContentClause(ctx, arg)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM conversation_claim c
 JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
 WHERE c.id = $%d AND c.archived_at IS NULL AND %s)`, arg(id), content)
	var visible bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&visible); err != nil {
		return fmt.Errorf("read the claim's evidence boundary: %w", err)
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}
