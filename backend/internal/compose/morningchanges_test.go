// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

func TestTheMorningOverlapCoversEveryCorrectionJobsTimeout(t *testing.T) {
	t.Parallel()
	overlap := morningChangeOverlap()
	for _, kind := range morningCorrectionJobs {
		spec, ok := jobs.SpecFor(kind)
		if !ok {
			t.Fatalf("%s is not a declared job, so its timeout is not in the overlap", kind)
		}
		if timeout := spec.Timeout.Duration(); timeout <= 0 || timeout > overlap {
			t.Errorf("%s may hold a transaction open %s, longer than the overlap %s", kind, timeout, overlap)
		}
	}
	sent := time.Date(2026, 11, 5, 7, 0, 0, 0, time.UTC)
	if got := morningChangesSince(&sent, sent.Add(time.Hour)); got != sent.Add(-overlap) {
		t.Errorf("window after a send at %s starts %s, want %s", sent, got, sent.Add(-overlap))
	}
}
