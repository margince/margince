// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package compose

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// The reporting pilot uses PERF-7's 300ms server assembly ceiling.
// This explicit benchmark measures 10,000 deals; it is not a merge-lane timing assertion.
func TestReportingEvaluationPilotBudget(t *testing.T) {
	f := reportingBusiness(t)
	f.at = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for i := range 9965 {
		f.createDeal(t, fmt.Sprintf("Volume %05d", i), 10000, f.env.stages[20], f.env.Rep1)
	}
	if _, err := f.env.owner.Exec(f.human, "ANALYZE deal; ANALYZE deal_stage_history"); err != nil {
		t.Fatal(err)
	}
	samples := make([]time.Duration, 0, 20)
	for i := range 23 {
		start := time.Now()
		result, err := f.service.Evaluate(f.human, f.selection())
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Charts) != 4 {
			t.Fatal("volume evaluation omitted charts")
		}
		if i >= 3 {
			samples = append(samples, elapsed)
		}
	}
	slices.Sort(samples)
	p95 := samples[18]
	t.Logf("reporting evaluation: 10000 deals, four charts, 20 samples; p50=%s p95=%s max=%s budget=300ms", samples[9], p95, samples[19])
	if p95 >= 300*time.Millisecond {
		t.Fatalf("reporting evaluation exceeded the pilot server assembly budget: %s", p95)
	}
}
