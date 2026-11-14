// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"strings"
	"testing"
	"time"
)

// The answer test is the largest part of the current-task clause, and Postgres
// keeps a plan of every copy on each pooled connection. A request and a lead
// cue lapse on the same answer, so the clause asks it once.
func TestTheCurrentTaskClauseAsksWhetherTheSourceWasAnsweredOnce(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	asOf := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	for name, clause := range map[string]string{
		"current":      CurrentTaskClause("a", asOf, "'[]'::jsonb", arg),
		"to line only": CurrentTaskClauseToLineOnly("a", asOf, "'[]'::jsonb", arg),
	} {
		if got := strings.Count(clause, "FROM activity answer_thread"); got != 1 {
			t.Errorf("the %s task clause carries %d answer tests, want 1", name, got)
		}
	}
}
