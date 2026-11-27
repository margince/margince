// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"testing"
	"time"
)

// TestANegativeProviderCountLowersNoTotal meters a call with a negative count
// after a real one. The day's totals keep the real call's tokens, and the
// negative call's own row reads zero, so the budget never moves down.
func TestANegativeProviderCountLowersNoTotal(t *testing.T) {
	t.Parallel()
	ledger := newSpendLedger(t, setupRateStore(t))
	day := time.Date(2026, 7, 10, 9, 0, 0, 0, time.UTC)
	ledger.at(day).call(TaskSummarize, TierCheapCloud, spendUnpricedModel, 100, 50, false)
	ledger.at(day.Add(time.Hour)).call(TaskSummarize, TierCheapCloud, spendUnpricedModel, -40, -10, false)

	var calls, tokensIn, tokensOut int64
	if err := ledger.env.owner.QueryRow(context.Background(),
		`SELECT calls, tokens_in, tokens_out FROM ai_usage WHERE day = $1 AND task = $2 AND tier = $3`,
		day, string(TaskSummarize), string(TierCheapCloud)).Scan(&calls, &tokensIn, &tokensOut); err != nil {
		t.Fatalf("reading the day's usage: %v", err)
	}
	if calls != 2 || tokensIn != 100 || tokensOut != 50 {
		t.Errorf("the day's usage reads calls=%d tokens_in=%d tokens_out=%d, want 2, 100 and 50",
			calls, tokensIn, tokensOut)
	}

	var negativeRows int
	if err := ledger.env.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM ai_call WHERE occurred_at = $1 AND tokens_in = 0 AND tokens_out = 0`,
		day.Add(time.Hour)).Scan(&negativeRows); err != nil {
		t.Fatalf("reading the negative call's row: %v", err)
	}
	if negativeRows != 1 {
		t.Errorf("found %d zero-count rows for the negative call, want its one row stored as zero", negativeRows)
	}
}
