// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strings"
	"testing"
)

// The column's CHECK admits what the sweep writes, and nothing else.
//
// The outcomes are spelled in Go and in a constraint. One in Go the constraint
// refuses is a write that fails at runtime, inside the transaction that stamps
// a considered original. One the constraint allows and no path writes is a
// reason nothing can produce.
func TestTheOutcomeCheckAdmitsEveryOutcomeTheSweepWrites(t *testing.T) {
	var def string
	err := ownerConn(t).QueryRow(context.Background(), `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		 WHERE conrelid = 'raw_capture'::regclass
		   AND conname = 'raw_capture_parts_slim_outcome_known'`).Scan(&def)
	if err != nil {
		t.Fatalf("reading the outcome constraint: %v", err)
	}
	for _, outcome := range partSlimOutcomes() {
		if !strings.Contains(def, string(outcome)) {
			t.Errorf("the constraint refuses %q, which the sweep writes", outcome)
		}
	}
	// And the other way, so a reason the constraint allows is one some path
	// can reach. Counted rather than parsed: the definition carries one quoted
	// literal per admitted value.
	if got, want := strings.Count(def, "'::text"), len(partSlimOutcomes()); got != want {
		t.Errorf("the constraint admits %d reasons, and the sweep writes %d", got, want)
	}
}
