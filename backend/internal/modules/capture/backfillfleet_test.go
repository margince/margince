// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics"
)

// The fleet gauge renders a zero for every status the column admits, so the
// list it renders from has to be the column's, read from the head catalog a
// fresh installation is migrated to.
func TestBackfillStatusesAreTheColumnsConstraint(t *testing.T) {
	catalog, err := os.ReadFile("../../../migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	var check string
	for line := range strings.SplitSeq(string(catalog), "\n") {
		if strings.HasPrefix(line, "public.capture_backfill.capture_backfill_status_check ") {
			check = line
		}
	}
	if check == "" {
		t.Fatal("the head catalog records no capture_backfill status CHECK")
	}
	var constrained []string
	for _, m := range regexp.MustCompile(`'([a-z_]+)'::text`).FindAllStringSubmatch(check, -1) {
		constrained = append(constrained, m[1])
	}
	if got, want := slices.Sorted(slices.Values(BackfillStatuses)), slices.Sorted(slices.Values(constrained)); !slices.Equal(got, want) {
		t.Errorf("BackfillStatuses = %v, the column admits %v", got, want)
	}
}

// A message the sink stored without tracing a decision is counted as the
// trace's own captured, so the two never read as separate outcomes.
func TestTheUntracedCaptureIsTheTracesCaptured(t *testing.T) {
	if capturemetrics.OutcomeCaptured != string(TraceCaptured) {
		t.Errorf("capturemetrics.OutcomeCaptured = %q, the trace says %q", capturemetrics.OutcomeCaptured, TraceCaptured)
	}
}

func TestLiveProgressSumsEveryCounter(t *testing.T) {
	got := BackfillProgress{Scanned: 1, Captured: 2, Skipped: 3, TotalEstimate: 4}.
		plus(BackfillProgress{Scanned: 10, Captured: 20, Skipped: 30, TotalEstimate: 40})
	if want := (BackfillProgress{Scanned: 11, Captured: 22, Skipped: 33, TotalEstimate: 44}); got != want {
		t.Errorf("plus = %+v, want %+v", got, want)
	}
}
