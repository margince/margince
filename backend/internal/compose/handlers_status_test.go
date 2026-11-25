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

func TestHeadStatusAnswersGetsStatusAndHeaders(t *testing.T) {
	srv := httptest.NewServer(contractRouter(Server{}))
	defer srv.Close()
	answers := map[string]*http.Response{}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req, err := http.NewRequestWithContext(t.Context(), method, srv.URL+"/v1/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Errorf("%s: closing the body: %v", method, err)
		}
		answers[method] = resp
	}
	get, head := answers[http.MethodGet], answers[http.MethodHead]
	if head.StatusCode != http.StatusOK {
		t.Fatalf("HEAD = %d, want 200", head.StatusCode)
	}
	for _, header := range []string{"Cache-Control", "Content-Type"} {
		if head.Header.Get(header) != get.Header.Get(header) {
			t.Errorf("HEAD sent %s %q, GET %q", header, head.Header.Get(header), get.Header.Get(header))
		}
	}
}
