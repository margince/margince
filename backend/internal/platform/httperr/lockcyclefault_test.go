// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// A lock cycle that outlasted the store's retries is a moment of contention,
// answered as retryable rather than as the opaque 500 of an unknown fault.
func TestALockCycleIsARetryableContention(t *testing.T) {
	for _, code := range []string{"40P01", "40001"} {
		t.Run(code, func(t *testing.T) {
			cause := fmt.Errorf("logging an activity: %w", &pgconn.PgError{Code: code, Message: "deadlock detected"})
			fault, ok := Classify(cause)
			if !ok || fault.Status != http.StatusServiceUnavailable || !fault.Transient() {
				t.Fatalf("classified as %+v (ok=%v), want a transient 503", fault, ok)
			}
			if fault.InfraCause == nil || strings.Contains(fault.Detail, "deadlock detected") {
				t.Errorf("the database's text must reach the log, not the caller: %q", fault.Detail)
			}

			rec := httptest.NewRecorder()
			Write(rec, httptest.NewRequest(http.MethodPost, "/v1/activities", nil), cause)
			if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") != "1" {
				t.Errorf("wrote %d with Retry-After %q, want 503 with Retry-After 1",
					rec.Code, rec.Header().Get("Retry-After"))
			}
		})
	}
}
