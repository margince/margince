// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// CountOpenDedupeCandidates is how many open pairs this caller can see. It
// shares the list's visibility clause, so the badge never counts a pair the
// list would hide. Entity types narrow it as they narrow the list.
func (s *Store) CountOpenDedupeCandidates(ctx context.Context, entityTypes ...string) (int, error) {
	return s.CountOpenDedupePairsOf(ctx, nil, entityTypes...)
}

// CountOpenDedupePairsOf is CountOpenDedupeCandidates for the pairs an owner
// set decides, or every visible pair when owners is nil.
func (s *Store) CountOpenDedupePairsOf(ctx context.Context, owners *storekit.OwnerSet, entityTypes ...string) (int, error) {
	if err := requireDedupeReadOf(ctx, entityTypes); err != nil {
		return 0, err
	}
	query := `SELECT count(*) FROM dedupe_candidate WHERE disposition = $1 AND archived_at IS NULL`
	args := []any{dispositionOpen}
	arg := func(v any) int { args = append(args, v); return len(args) }
	visClause, err := dedupeVisibilityClause(ctx, arg, true)
	if err != nil {
		return 0, err
	}
	if visClause != "" {
		query += " AND " + visClause
	}
	if len(entityTypes) > 0 {
		query += fmt.Sprintf(" AND entity_type = ANY($%d)", arg(entityTypes))
	}
	if clause := dedupePairOwnerClause(owners, arg); clause != "" {
		query += " AND " + clause
	}
	var open int
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&open)
	}); err != nil {
		return 0, fmt.Errorf("contacts: counting open dedupe candidates: %w", err)
	}
	return open, nil
}
