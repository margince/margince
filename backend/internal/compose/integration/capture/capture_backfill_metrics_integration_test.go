// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func pageSample(t *testing.T, result string) float64 {
	t.Helper()
	series := `margince_capture_backfill_pages_total{provider="gmail",result="` + result + `"} `
	var b strings.Builder
	capturemetrics.WriteProcessMetrics(&b)
	for line := range strings.SplitSeq(b.String(), "\n") {
		if value, ok := strings.CutPrefix(line, series); ok {
			f, err := strconv.ParseFloat(value, 64)
			if err != nil {
				t.Fatalf("%s carries an unparseable value %q", series, value)
			}
			return f
		}
	}
	return 0
}

// The pager names the run's provider from the connection it read, so a page is
// counted under it by result, and the job's own run learns the provider too.
func TestEveryBackfillPageIsCountedUnderItsProviderByResult(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, runID := startFlakyBackfill(t, e, []error{&connector.RateLimitedError{}})
	jobCtx := capturemetrics.WithRun(principal.WithWorkspaceID(context.Background(), e.WS))
	limitedBefore, okBefore := pageSample(t, "rate_limited"), pageSample(t, "ok")

	if _, _, retryAfter, _ := registry.RunBackfillStep(jobCtx, runID); retryAfter <= 0 {
		t.Fatalf("retryAfter = %v, want the rate-limited page waited out", retryAfter)
	}
	if _, _, _, err := registry.RunBackfillStep(jobCtx, runID); err != nil {
		t.Fatalf("the recovered page: %v", err)
	}

	if moved := pageSample(t, "rate_limited") - limitedBefore; moved != 1 {
		t.Errorf("rate_limited pages moved by %v, want 1", moved)
	}
	if moved := pageSample(t, "ok") - okBefore; moved != 1 {
		t.Errorf("ok pages moved by %v, want 1", moved)
	}

	var b strings.Builder
	capturemetrics.ObservePacing(jobCtx, 0)
	capturemetrics.WriteProcessMetrics(&b)
	if !strings.Contains(b.String(), `margince_capture_backfill_snooze_seconds_total{provider="gmail",reason="pacing"}`) {
		t.Errorf("the job's run never learned the provider the pager read:\n%s", b.String())
	}
}
