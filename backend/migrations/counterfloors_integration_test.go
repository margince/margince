// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// TestACounterBelowZeroIsRefused writes a negative value into every token
// counter and into an activity's duration, and expects the floor named for
// that column to refuse it.
func TestACounterBelowZeroIsRefused(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	type floor struct{ name, sql, constraint string }
	var floors []floor
	for _, column := range []string{"calls", "cached_hits", "tokens_in", "tokens_out"} {
		floors = append(floors, floor{
			name:       "ai_usage." + column,
			sql:        fmt.Sprintf(`INSERT INTO ai_usage (day, task, tier, %s) VALUES (current_date, 'summarize', 'cheap_cloud', -1)`, column),
			constraint: "ai_usage_" + column + "_not_negative",
		})
	}
	for _, column := range []string{"tokens_in", "tokens_out", "reasoning_tokens", "cached_tokens", "cache_write_tokens"} {
		floors = append(floors, floor{
			name:       "ai_call." + column,
			sql:        fmt.Sprintf(`INSERT INTO ai_call (logical_call_id, task, %s) VALUES (gen_random_uuid(), 'summarize', -1)`, column),
			constraint: "ai_call_" + column + "_not_negative",
		})
	}
	floors = append(floors, floor{
		name:       "activity.duration_seconds",
		sql:        `INSERT INTO activity (kind, source, captured_by, duration_seconds) VALUES ('call', 'manual', 'human:test', -1)`,
		constraint: "activity_duration_seconds_not_negative",
	})

	for _, f := range floors {
		t.Run(f.name, func(t *testing.T) {
			if got := refusingConstraint(t, conn, f.sql); got != f.constraint {
				t.Errorf("the row was refused by %q, want %q", got, f.constraint)
			}
		})
	}
}

// TestADayTotalBelowZeroIsRaisedToZeroBeforeItsFloorIsValidated stores a day
// total a negative count lowered below zero, then migrates past the validation.
// The total reads zero, the value the meter now stores, and the upgrade passes.
func TestADayTotalBelowZeroIsRaisedToZeroBeforeItsFloorIsValidated(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, namespaceThroughVersion(t, core, "1795675788")); err != nil {
		t.Fatalf("migrating to the floors: %v", err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE ai_usage DROP CONSTRAINT ai_usage_tokens_in_not_negative`); err != nil {
		t.Fatalf("lifting the floor to store an older total: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO ai_usage (day, task, tier, calls, tokens_in, tokens_out)
		VALUES (current_date, 'summarize', 'cheap_cloud', 2, -40, 50)`); err != nil {
		t.Fatalf("seeding a total below zero: %v", err)
	}
	if _, err := conn.Exec(ctx, `ALTER TABLE ai_usage ADD CONSTRAINT ai_usage_tokens_in_not_negative
		CHECK (tokens_in >= 0) NOT VALID`); err != nil {
		t.Fatalf("restoring the floor: %v", err)
	}

	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("the validation refused the upgrade: %v", err)
	}
	var tokensIn, tokensOut int64
	if err := conn.QueryRow(ctx, `SELECT tokens_in, tokens_out FROM ai_usage WHERE task = 'summarize'`).
		Scan(&tokensIn, &tokensOut); err != nil {
		t.Fatalf("reading the total: %v", err)
	}
	if tokensIn != 0 || tokensOut != 50 {
		t.Errorf("the total reads tokens_in=%d tokens_out=%d, want 0 and the untouched 50", tokensIn, tokensOut)
	}
}
