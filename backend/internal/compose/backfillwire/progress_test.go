// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backfillwire_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/backfillwire"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestCourseReportsARestartedScanAndARetryApart(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                   string
		run                    capture.BackfillRun
		wantRestarts, wantTrys int
	}{
		{"a restarted scan", capture.BackfillRun{WindowRestarts: 1, RetryAttempts: 1}, 1, 1},
		{"a retried page", capture.BackfillRun{RetryAttempts: 2}, 0, 2},
		{"a run that never stumbled", capture.BackfillRun{}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var st crmcontracts.BackfillStatus
			backfillwire.Course(&st, &tc.run)
			if st.WindowRestarts == nil || *st.WindowRestarts != tc.wantRestarts {
				t.Errorf("window_restarts = %v, want %d", st.WindowRestarts, tc.wantRestarts)
			}
			if st.RetryAttempts == nil || *st.RetryAttempts != tc.wantTrys {
				t.Errorf("retry_attempts = %v, want %d", st.RetryAttempts, tc.wantTrys)
			}
		})
	}
}
