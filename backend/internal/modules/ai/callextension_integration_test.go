// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The extension column reaches the table through the real writer, and a core
// call leaves it empty.
func TestACallRecordsTheExtensionItWasMadeFor(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	meter := NewCallMeter(db)

	extensionCall, coreCall := ids.NewV7(), ids.NewV7()
	calls := []Call{
		{
			LogicalCallID: extensionCall, Attempt: 1, IsTerminal: true, Kind: callKindCompletion,
			Task: TaskExtensionTask, Tier: TierCheapCloud, RequestFingerprint: "fp-extension", Extension: "books/summarise",
		},
		{
			LogicalCallID: coreCall, Attempt: 1, IsTerminal: true, Kind: callKindCompletion,
			Task: TaskSummarize, Tier: TierCheapCloud, RequestFingerprint: "fp-core",
		},
	}
	for _, c := range calls {
		if err := meter.Record(ctx, []Call{c}); err != nil {
			t.Fatalf("recording %s: %v", c.Task, err)
		}
	}

	for logical, want := range map[ids.UUID]string{extensionCall: "books/summarise", coreCall: ""} {
		var got string
		err := db.Pool().QueryRow(ctx, `SELECT extension FROM ai_call WHERE logical_call_id = $1`, logical).Scan(&got)
		if err != nil {
			t.Fatalf("reading the row back: %v", err)
		}
		if got != want {
			t.Errorf("extension reads %q, want %q", got, want)
		}
	}
}
