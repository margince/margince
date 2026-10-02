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
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %s, want exactly {\"status\":\"ok\"}: monitors string-match it", got)
	}
}
