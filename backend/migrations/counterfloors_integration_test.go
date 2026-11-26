// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"fmt"
	"testing"
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
