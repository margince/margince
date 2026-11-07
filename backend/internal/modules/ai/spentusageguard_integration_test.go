// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSpentTokensAndTheirTraceSurviveExecutionOwnershipLoss(t *testing.T) {
	env := setupRateStore(t)
	ws, ctx := env.seedWorkspace(context.Background(), t)
	lost := errors.New("execution ownership was lost")
	guarded := database.WithTransactionGuard(ctx, func(context.Context, pgx.Tx) error { return lost })
	db := env.dbFor(ws)
	called := false
	if err := db.Tx(guarded, func(pgx.Tx) error { called = true; return nil }); !errors.Is(err, lost) || called {
		t.Fatalf("domain work escaped its guard: called=%v err=%v", called, err)
	}
	meter := NewMeter(db)
	meter.now = func() time.Time { return time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC) }
	if err := meter.Record(guarded, Usage{Task: TaskSummarize, Tier: TierCheapCloud, TokensIn: 7, TokensOut: 6}); err != nil {
		t.Fatal(err)
	}
	tokens, err := meter.MonthTokens(ctx)
	if err != nil || tokens != 13 {
		t.Fatalf("spent tokens=%d err=%v", tokens, err)
	}
	logical := ids.NewV7()
	call := traceCall("", 7, 6, "spent-model")
	call.LogicalCallID, call.IsTerminal = logical, true
	router := &Router{calls: NewCallMeter(db), log: slog.New(slog.DiscardHandler)}
	router.flushDetached(guarded, &binding{}, &logicalCall{attempts: []Call{call}})
	var traced int
	err = env.owner.QueryRow(ctx, `SELECT coalesce(sum(tokens_in + tokens_out), 0) FROM ai_call WHERE logical_call_id = @logical`, pgx.StrictNamedArgs{"logical": logical}).Scan(&traced)
	if err != nil || traced != 13 {
		t.Fatalf("trace tokens=%d err=%v", traced, err)
	}
}
