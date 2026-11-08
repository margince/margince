// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
)

// An agent refused on its allowance is told which limit stopped it, the
// ceiling, what it used and when to come back, in fields and in the header.
func TestAnAllowanceRefusalNamesItsLimitAndWhenToRetry(t *testing.T) {
	resets := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	refusal := &auth.VolumeExceededError{Reading: agentvolume.Reading{
		Counter: agentvolume.Calls, Observed: 1000, Limit: 1000, Exceeded: true,
		ResetsAt: resets, ResetsIn: 56*time.Minute + 500*time.Millisecond,
	}}
	rec := httptest.NewRecorder()

	Write(rec, httptest.NewRequest(http.MethodGet, "/v1/contacts", nil), refusal)

	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3361" {
		t.Fatalf("answered %d with Retry-After %q, want 429 and 3361", rec.Code, rec.Header().Get("Retry-After"))
	}
	var body struct {
		Code    string `json:"code"`
		Detail  string `json:"detail"`
		Details struct {
			Limit struct {
				Kind     string `json:"kind"`
				Ceiling  int    `json:"ceiling"`
				Used     int    `json:"used"`
				ResetsAt string `json:"resets_at"`
			} `json:"limit"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	limit := body.Details.Limit
	if body.Code != "rate_limited" || limit.Kind != "calls" || limit.Ceiling != 1000 || limit.Used != 1000 ||
		limit.ResetsAt != "2026-11-09T00:00:00Z" {
		t.Errorf("the body is %+v", body)
	}
	if !strings.Contains(body.Detail, "2026-11-09T00:00:00Z") {
		t.Errorf("the sentence does not say when the window rolls: %q", body.Detail)
	}
}
