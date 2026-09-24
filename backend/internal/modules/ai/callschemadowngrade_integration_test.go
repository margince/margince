// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Every value an adapter reports for how much of the response schema it sent
// survives the real write path and the column's CHECK, and an unknown value is
// refused rather than stored as a claim nothing produces.
func TestTheCallTraceRecordsWhetherGenerationHeldTheSchema(t *testing.T) {
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	meter := NewCallMeter(env.dbFor(ws))

	record := func(downgrade string) (ids.UUID, error) {
		logical := ids.NewV7()
		return logical, meter.Record(ctx, []Call{{
			LogicalCallID: logical, Attempt: 1, IsTerminal: true,
			Kind: callKindCompletion, Task: TaskSummarize, Tier: TierPremium,
			Provider: providerAnthropic, ModelID: "claude-sonnet-4-6",
			RequestFingerprint: "fp-schema-" + downgrade, SchemaDowngrade: downgrade,
		}})
	}
	for _, downgrade := range model.SchemaDowngrades() {
		logical, err := record(downgrade)
		if err != nil {
			t.Fatalf("recording a %q downgrade: %v", downgrade, err)
		}
		var stored string
		if err := env.owner.QueryRow(ctx,
			`SELECT schema_downgrade FROM ai_call WHERE logical_call_id = $1`, logical,
		).Scan(&stored); err != nil {
			t.Fatalf("reading the call back: %v", err)
		}
		if stored != downgrade {
			t.Errorf("schema_downgrade = %q, want %q", stored, downgrade)
		}
	}
	_, err := record("partially")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != "ai_call_schema_downgrade_check" {
		t.Errorf("a downgrade no adapter reports must be refused by the column's CHECK, got %v", err)
	}
}
