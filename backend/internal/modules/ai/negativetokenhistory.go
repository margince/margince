// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// negativeTokenCall holds while an ai_call row stores a token figure below
// zero. It is the predicate of idx_ai_call_negative_tokens.
const negativeTokenCall = `(tokens_in < 0 OR tokens_out < 0 OR reasoning_tokens < 0 OR cached_tokens < 0 OR cache_write_tokens < 0)`

// CountNegativeTokenCalls counts the calls still storing a negative token
// figure, up to maximum plus one.
func CountNegativeTokenCalls(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var left int64
	if err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM ai_call WHERE `+negativeTokenCall+` LIMIT $1) negative`,
		maximum+1).Scan(&left); err != nil {
		return 0, false, fmt.Errorf("ai: counting calls with a negative token count: %w", err)
	}
	return min(left, maximum), left > maximum, nil
}

// ZeroNegativeTokenCountsTx raises each negative token figure on the next
// page of calls to zero, the figure the meter stores for one today. A raised
// row leaves the page, so a replay finds nothing to change.
func ZeroNegativeTokenCountsTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	var page []ids.UUID
	if err := tx.QueryRow(ctx, `WITH page AS (
		  SELECT id FROM ai_call WHERE `+negativeTokenCall+` AND id > $1 ORDER BY id LIMIT $2 FOR UPDATE),
		raised AS (UPDATE ai_call c
		      SET tokens_in = greatest(c.tokens_in, 0), tokens_out = greatest(c.tokens_out, 0),
		          reasoning_tokens = greatest(c.reasoning_tokens, 0), cached_tokens = greatest(c.cached_tokens, 0),
		          cache_write_tokens = greatest(c.cache_write_tokens, 0)
		     FROM page WHERE c.id = page.id RETURNING c.id)
		SELECT coalesce(array_agg(id ORDER BY id), '{}') FROM raised`, after, in.Limit).Scan(&page); err != nil {
		return datatask.BatchOut{}, fmt.Errorf("ai: raising negative token counts to zero: %w", err)
	}
	return datatask.AdvanceUUID(in, page, len(page))
}
