// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetStatusAnswersAFixedUncachedBody(t *testing.T) {
	rec := httptest.NewRecorder()
	Server{}.GetStatus(rec, httptest.NewRequest(http.MethodGet, "/v1/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store: a cached answer would hide an outage", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	// The exact wire bytes, newline included: monitors match on this text, so
	// the test must see what they see rather than a trimmed copy of it.
	if got := rec.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q, want exactly %q", got, "{\"status\":\"ok\"}\n")
	}
}
