// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"log/slog"
)

// storedTokenCount is the count a meter writes for one provider-reported
// figure. A negative count is stored as zero and logged: the call was served
// and spent, so refusing it saves nothing, while adding it would lower the
// totals the budget reads. The schema's non-negative checks hold the result.
func storedTokenCount(ctx context.Context, log *slog.Logger, column string, reported int) int {
	if reported >= 0 {
		return reported
	}
	log.WarnContext(ctx, "ai: a provider reported a negative token count, stored as zero",
		"column", column, "reported", reported)
	return 0
}

// withStoredTokenCounts is c with every token figure passed through
// storedTokenCount, the form recordAttempts writes to ai_call.
func withStoredTokenCounts(ctx context.Context, log *slog.Logger, c Call) Call {
	c.TokensIn = storedTokenCount(ctx, log, "tokens_in", c.TokensIn)
	c.TokensOut = storedTokenCount(ctx, log, "tokens_out", c.TokensOut)
	c.ReasoningTokens = storedTokenCount(ctx, log, "reasoning_tokens", c.ReasoningTokens)
	c.CachedTokens = storedTokenCount(ctx, log, "cached_tokens", c.CachedTokens)
	c.CacheWriteTokens = storedTokenCount(ctx, log, "cache_write_tokens", c.CacheWriteTokens)
	return c
}
