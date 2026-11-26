// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"log/slog"
)

// usageSource names where a token figure came from, so a warning tells an
// operator which provider or task misreports.
type usageSource struct {
	task     Task
	tier     Tier
	provider string
	model    string
}

// storedTokenCount is the count a meter writes for one provider-reported
// figure. A negative count is stored as zero and logged. The call was served
// and spent, so refusing it saves nothing. Adding it would lower the totals the
// budget reads. The schema's non-negative checks hold the result.
func storedTokenCount(ctx context.Context, log *slog.Logger, source usageSource, column string, reported int) int {
	if reported >= 0 {
		return reported
	}
	log.WarnContext(ctx, "ai: a provider reported a negative token count, stored as zero; check the provider's usage figures",
		"column", column, "reported", reported, "task", string(source.task), "tier", string(source.tier),
		"provider", source.provider, "model", source.model)
	return 0
}

// withStoredTokenCounts is c with every token figure passed through
// storedTokenCount, the form recordAttempts writes to ai_call.
func withStoredTokenCounts(ctx context.Context, log *slog.Logger, c Call) Call {
	source := usageSource{task: c.Task, tier: c.Tier, provider: c.Provider, model: c.ModelID}
	c.TokensIn = storedTokenCount(ctx, log, source, "tokens_in", c.TokensIn)
	c.TokensOut = storedTokenCount(ctx, log, source, "tokens_out", c.TokensOut)
	c.ReasoningTokens = storedTokenCount(ctx, log, source, "reasoning_tokens", c.ReasoningTokens)
	c.CachedTokens = storedTokenCount(ctx, log, source, "cached_tokens", c.CachedTokens)
	c.CacheWriteTokens = storedTokenCount(ctx, log, source, "cache_write_tokens", c.CacheWriteTokens)
	return c
}
