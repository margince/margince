// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"testing"
	"time"
)

func TestHealthAssessmentDoesNotTreatHistoryAsRecovery(t *testing.T) {
	future := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		row       KindHealth
		condition string
	}{
		{"cancelled deliberately", KindHealth{Cancelled: 1, Dead: 1}, "unknown"},
		{"discarded work remains", KindHealth{Discarded: 1}, "unverified"},
		{"retry recorded", KindHealth{Retrying: 1, FutureRetries: 1, NextAttemptAt: &future}, "retrying"},
		{"future scheduled", KindHealth{Waiting: 1}, "processing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AssessHealth(tc.row); got.Condition != tc.condition {
				t.Fatalf("got %s, want %s", got.Condition, tc.condition)
			}
		})
	}
}

func TestAQueueWithoutAnExpectationCannotBeCalledOnTime(t *testing.T) {
	age := 600.0
	got := AssessHealth(KindHealth{Kind: "unknown", Waiting: 1, OldestWaitingAgeSeconds: &age})
	if got.Condition != "unknown" || got.Expectation != 0 {
		t.Fatalf("unexpected assessment: %+v", got)
	}
}

func TestRunnableWaitIsOverdueOnlyPastItsDeclaredTick(t *testing.T) {
	tested := 0
	for kind, spec := range Declared() {
		if spec.Cadence.Fixed <= 0 {
			continue
		}
		tested++
		t.Run(kind, func(t *testing.T) {
			age := spec.Cadence.Fixed.Seconds()
			row := KindHealth{Kind: kind, Waiting: 1, OldestWaitingAgeSeconds: &age}
			if AssessHealth(row).Code == "queue_overdue" {
				t.Fatal("boundary is not overdue")
			}
			age++
			if AssessHealth(row).Code != "queue_overdue" {
				t.Fatal("missed tick must be overdue")
			}
		})
	}
	if tested > 0 {
		return
	}
	t.Fatal("no declared periodic kind tested")
}
