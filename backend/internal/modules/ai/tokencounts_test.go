// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestANegativeTokenFigureIsStoredAsZeroAndLogged(t *testing.T) {
	var logged bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logged, nil))
	got := withStoredTokenCounts(context.Background(), log, Call{
		Provider: "anthropic", ModelID: "claude-test", TokensIn: -3, TokensOut: 7, ReasoningTokens: -1, CachedTokens: 2, CacheWriteTokens: -9,
	})
	want := Call{TokensIn: 0, TokensOut: 7, ReasoningTokens: 0, CachedTokens: 2, CacheWriteTokens: 0}
	if got.TokensIn != want.TokensIn || got.TokensOut != want.TokensOut || got.ReasoningTokens != want.ReasoningTokens ||
		got.CachedTokens != want.CachedTokens || got.CacheWriteTokens != want.CacheWriteTokens {
		t.Errorf("stored %+v, want the negative figures as zero and the rest kept", got)
	}
	for _, column := range []string{"tokens_in", "reasoning_tokens", "cache_write_tokens"} {
		if !strings.Contains(logged.String(), "column="+column) {
			t.Errorf("no warning names %s, which was negative:\n%s", column, logged.String())
		}
	}
	if !strings.Contains(logged.String(), "provider=anthropic model=claude-test") {
		t.Errorf("the warning does not name the provider and model that misreported:\n%s", logged.String())
	}
	for _, column := range []string{"tokens_out", "cached_tokens"} {
		if strings.Contains(logged.String(), "column="+column+" ") {
			t.Errorf("a warning names %s, which was not negative:\n%s", column, logged.String())
		}
	}
}
