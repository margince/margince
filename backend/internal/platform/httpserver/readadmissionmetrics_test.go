// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
)

func TestReadAdmissionPublishesWaitOutsidePoolCheckout(t *testing.T) {
	rec := httptest.NewRecorder()
	out := &exposition{w: rec}
	writeReadAdmissionMetrics(out, database.ReadAdmissionCost{
		Attempts: 7, Wait: 3 * time.Second, MaxWait: 2 * time.Second, Waiters: 4, PeakWaiters: 5, Active: 2,
	})
	for _, want := range []string{
		"# TYPE margince_read_admission_attempts_total counter\nmargince_read_admission_attempts_total 7",
		"# TYPE margince_read_admission_wait_seconds_total counter\nmargince_read_admission_wait_seconds_total 3",
		"margince_read_admission_wait_seconds_max 2", "margince_read_admission_waiters 4",
		"margince_read_admission_waiters_peak 5", "margince_read_admission_active 2",
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing metric %q from %s", want, rec.Body.String())
		}
	}
}
