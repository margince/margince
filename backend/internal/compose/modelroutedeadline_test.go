// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

// A handler that CALLS a model needs longer to answer than the server's
// WriteTimeout allows every other endpoint, and it takes that time on its own
// route rather than from the server.
//
// Measured on a real installation, a reply draft runs 13 to 45 seconds and the
// server-wide WriteTimeout is 30s: the response was cut mid-write and reached
// the reader as a 502 from the proxy in front. Raising the global bound would
// have let one slow-reading client hold a connection for as long as it liked,
// which is the trade the MCP handler already refused.
func TestOnlyTheModelRoutesTakeTheLongerDeadline(t *testing.T) {
	// A ResponseController reaches the deadline through the writer; a recorder
	// does not implement it, so the middleware's refusal path is what a test
	// without a real server sees. The route DECISION is what is asserted here,
	// and it is the half that decides which endpoints are exposed.
	for path, wantsModelTime := range map[string]bool{
		"/v1/activities/01a0-4cd3/draft-email":            true,
		"/v1/organizations/01a0-4cd2/dossier":             true,
		"/v1/organizations/01a0-4cd2/growth-fit":          true,
		"/v1/activities/01a0-4cd3/meeting-brief":          true,
		"/v1/organizations/01a0-4cd2/ask":                 true,
		"/v1/knowledge/corpora/01a0-4cd2/ask":             true,
		"/v1/people/01a0-4cd2/intro-note-draft":           true,
		"/v1/organizations/01a0-4cd2/intro-request-draft": true,
		"/v1/deals/01a0-4cd2/role-proposals":              true,
		"/v1/people":                                      false,
		"/v1/me":                                          false,
		"/v1/organizations/01a0-4cd2":                     false,
		// A path that merely CONTAINS a slow route's name is not one: the
		// suffix is the whole match, or a list endpoint beside it inherits a
		// deadline it has no work for.
		"/v1/draft-email/history": false,
	} {
		if got := callsAModel(path); got != wantsModelTime {
			t.Errorf("callsAModel(%q) = %v, want %v", path, got, wantsModelTime)
		}
	}
}

// The deadline covers the whole logical call, not one request to the provider:
// the router may spend its per-call ceiling on every rung of the ladder, and
// may walk that ladder more than once for a single answer. Sized for one call,
// a legitimate retry would be cut — the same defect one level down from the
// server timeout this replaces.
func TestTheRouteDeadlineCoversTheWholeLadder(t *testing.T) {
	if ai.RouteWriteDeadline <= ai.CallCeiling {
		t.Fatalf("route deadline %s must outlast one model call (%s)",
			ai.RouteWriteDeadline, ai.CallCeiling)
	}
	// Two rungs, walked more than once, is the shape the router can produce.
	if ai.RouteWriteDeadline < 2*ai.CallCeiling {
		t.Fatalf("route deadline %s cannot cover a ladder that falls back once (%s)",
			ai.RouteWriteDeadline, 2*ai.CallCeiling)
	}
}

// A route with no model behind it is handed straight through — the middleware
// must not touch the deadline of an ordinary read.
func TestAnOrdinaryRouteIsUntouched(t *testing.T) {
	served := false
	handler := extendDeadlineForModelRoutes(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		served = true
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/me", nil))

	if !served {
		t.Fatal("an ordinary route is served, deadline untouched")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

// And a chain that cannot extend the deadline fails LOUDLY rather than serving
// a response that dies mid-write, which is the symptom this exists to remove.
func TestAChainThatCannotExtendSaysSo(t *testing.T) {
	handler := extendDeadlineForModelRoutes(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("drafted"))
	}))

	recorder := httptest.NewRecorder() // implements no ResponseController hook
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/activities/x/draft-email", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 — a chain that cannot extend must say so", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "deadline_not_extendable") {
		t.Fatalf("body = %q, want the machine code an operator can act on", body)
	}
}
