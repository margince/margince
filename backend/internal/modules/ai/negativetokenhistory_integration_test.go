// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// storedTokens reads a call's five token figures in column order.
func storedTokens(t *testing.T, e *rateEnv, call ids.UUID) []int64 {
	t.Helper()
	figures := make([]int64, 5)
	if err := e.owner.QueryRow(context.Background(), `SELECT tokens_in, tokens_out, reasoning_tokens, cached_tokens, cache_write_tokens
		 FROM ai_call WHERE id = $1`, call).Scan(&figures[0], &figures[1], &figures[2], &figures[3], &figures[4]); err != nil {
		t.Fatal(err)
	}
	return figures
}

// zeroNegativeTokens runs the data task's step to its end, one call a batch,
// and answers how many calls it changed.
func zeroNegativeTokens(t *testing.T, e *rateEnv, ws ids.UUID) int {
	t.Helper()
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), ws), "system:negative-counts")
	in, changed := datatask.BatchIn{Limit: 1}, 0
	for {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(system, e.pool, func(tx pgx.Tx) error {
			var err error
			out, err = ZeroNegativeTokenCountsTx(system, tx, in)
			return err
		}); err != nil {
			t.Fatalf("raising negative token counts: %v", err)
		}
		changed += out.Changed
		if out.Done {
			return changed
		}
		in.Cursor = out.Cursor
	}
}

func TestANegativeTokenFigureIsRaisedToZeroAndTheRestStay(t *testing.T) {
	e := setupRateStore(t)
	ws, _ := e.seedWorkspace(context.Background(), t)
	misreported, served := ids.NewV7(), ids.NewV7()
	testdb.WithNegativeCountFloorsLifted(t, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO ai_call (id, task, tier, provider, model_id, logical_call_id,
			  tokens_in, tokens_out, reasoning_tokens, cached_tokens, cache_write_tokens)
			VALUES ($1, 'draft_reply', 'cheap_cloud', 'fake', 'fake-1', $1, -5, 40, 0, -3, -1),
			       ($2, 'draft_reply', 'cheap_cloud', 'fake', 'fake-1', $2, 120, 60, 7, 30, 2)`, misreported, served)
		return err
	})

	if changed := zeroNegativeTokens(t, e, ws); changed != 1 {
		t.Errorf("the task changed %d calls, want the one with negative figures", changed)
	}
	if got := storedTokens(t, e, misreported); !slices.Equal(got, []int64{0, 40, 0, 0, 0}) {
		t.Errorf("the misreported call stores %v, want each negative figure at zero and 40 tokens out kept", got)
	}
	if got := storedTokens(t, e, served); !slices.Equal(got, []int64{120, 60, 7, 30, 2}) {
		t.Errorf("a call with no negative figure stores %v, want it untouched", got)
	}
	if changed := zeroNegativeTokens(t, e, ws); changed != 0 {
		t.Errorf("a replay changed %d calls, want none", changed)
	}
}
