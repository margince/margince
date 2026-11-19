// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
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

// Today holds an undated source's task with no date. The undated backlog
// holds only a user's. A source stating neither writes tasks no list shows.
func TestEveryAutomaticTaskSourceStatesWhetherItsTasksCarryADate(t *testing.T) {
	for _, source := range TaskSourceFamilies() {
		if slices.Contains(provenance.RetiredTaskSources(), source) {
			continue
		}
		policy := taskSourceFamilies[source]
		if policy.undated == policy.dated {
			t.Errorf("task source %s states undated=%t and dated=%t, want exactly one", source, policy.undated, policy.dated)
		}
	}
}
