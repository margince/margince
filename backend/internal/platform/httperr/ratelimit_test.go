// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestATimedRefusalCarriesHeadersThroughAnErrorWrapper(t *testing.T) {
	t.Parallel()
	limit := ratelimit.NewRegistry().New("test/request", ratelimit.FailClosed, 3, time.Minute)
	err := fmt.Errorf("request refused: %w", ratelimit.Refused(limit))
	rec := httptest.NewRecorder()
	Write(rec, httptest.NewRequest(http.MethodPost, "/v1/test", nil), err)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatal(rec.Code)
	}
	for name, want := range map[string]string{"RateLimit-Limit": "3", "RateLimit-Remaining": "0", "RateLimit-Reset": "60", "Retry-After": "60"} {
		if got := rec.Header().Get(name); got != want {
			t.Fatalf("%s=%q, want %q", name, got, want)
		}
	}
	fault, ok := Classify(err)
	if !ok || fault.RetryAfterSeconds != 60 {
		t.Fatalf("classified retry=%+v", fault)
	}
}

func TestAnUntimedBudgetRefusalMakesNoPacingPromise(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	Write(rec, httptest.NewRequest(http.MethodPost, "/v1/test", nil), apperrors.ErrBudgetExceeded)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatal(rec.Code)
	}
	for _, name := range []string{"RateLimit-Limit", "RateLimit-Remaining", "RateLimit-Reset", "Retry-After"} {
		if rec.Header().Get(name) != "" {
			t.Fatalf("invented %s", name)
		}
	}
}
