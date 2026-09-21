// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httpserver"
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
		"/v1/activities/01a0-4cd3/draft-email":        true,
		"/v1/companies/01a0-4cd2/dossier":             true,
		"/v1/companies/01a0-4cd2/growth-fit":          true,
		"/v1/activities/01a0-4cd3/meeting-brief":      true,
		"/v1/brief":                                   true,
		"/v1/contacts/01a0-4cd2/brief":                true,
		"/v1/companies/01a0-4cd2/brief":               true,
		"/v1/companies/01a0-4cd2/ask":                 true,
		"/v1/knowledge/corpora/01a0-4cd2/ask":         true,
		"/v1/contacts/01a0-4cd2/intro-note-draft":     true,
		"/v1/companies/01a0-4cd2/intro-request-draft": true,
		"/v1/deals/01a0-4cd2/role-proposals":          true,
		"/v1/companies/01a0-4cd2/enrich":              true,
		"/v1/coldstart":                               true,
		"/v1/coldstart/preview":                       true,
		"/v1/onboarding/company/messages":             true,
		"/v1/company/site-reads/01a0-4cd2/messages":   true,
		"/v1/offers/01a0-4cd2/regenerate":             true,
		"/v1/contacts":                                false,
		"/v1/me":                                      false,
		"/v1/companies/01a0-4cd2":                     false,
		// `/deals/{id}/status` also calls a model (`x-waits-on-model: on-miss`)
		// and has no safe suffix here: "/status" alone would also catch
		// /contracts/{id}/status, /embeddings/reindex/status and
		// /overlay/sync-status, none of which call a model, and this matcher
		// has no way to require the segment before it be "deals". Tracked as
		// margince#4914 rather than widened past what a bare suffix can say
		// safely; TestTheModelRouteDeadlineSuffixesCoverTheContract names this
		// one exception explicitly so the gap stays visible.
		"/v1/deals/01a0-4cd2/status":     false,
		"/v1/contracts/01a0-4cd2/status": false,
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

// A route with no model behind it keeps the server's own write deadline — the
// middleware must not hand an ordinary read the long one — and runs under a
// context bounded by that same figure.
//
// The context half is what stops a handler outliving the response it was
// writing. Until it, a read whose answer was already lost kept its goroutine
// and every database connection it took from there on, against a pool of 16
// that the worklist family asks 33-54 transactions of per request.
func TestAnOrdinaryRouteRunsUnderTheServersOwnDeadline(t *testing.T) {
	// Measured from INSIDE the handler: the deadline is set a hair before it
	// runs, so what the handler has left is what the bound actually gives it.
	var within time.Duration
	var bounded bool
	handler := boundByResponseDeadline(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var deadline time.Time
		deadline, bounded = r.Context().Deadline()
		within = time.Until(deadline)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/me", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — an ordinary route keeps the short write deadline", recorder.Code)
	}
	if !bounded {
		t.Fatal("the handler ran under a context with no deadline, so a lost response costs its connections until the work ends on its own")
	}
	if within > httpserver.ResponseDeadline || within < httpserver.ResponseDeadline-time.Second {
		t.Errorf("the handler had %s left, want the response's own %s",
			within, httpserver.ResponseDeadline)
	}
}

// And a model route's context is bounded by ITS deadline, not the server's.
//
// The two halves have to name the same figure per route or the change is a
// regression rather than a fix: bound at the server's, a draft that legitimately
// runs past 30s would be cancelled by the very middleware that exists to let it
// finish.
func TestAModelRouteRunsUnderTheDeadlineItsResponseWasGiven(t *testing.T) {
	var within time.Duration
	var bounded bool
	handler := boundByResponseDeadline(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var deadline time.Time
		deadline, bounded = r.Context().Deadline()
		within = time.Until(deadline)
	}))

	handler.ServeHTTP(&deadlineWriter{ResponseRecorder: httptest.NewRecorder()},
		httptest.NewRequest(http.MethodPost, "/v1/activities/01a0-4cd3/draft-email", nil))

	if !bounded {
		t.Fatal("the model route ran under a context with no deadline")
	}
	if within > ai.RouteWriteDeadline || within < ai.RouteWriteDeadline-time.Second {
		t.Errorf("the handler had %s left, want the route's own %s", within, ai.RouteWriteDeadline)
	}
	// The whole point of the per-route arm: it must be the LONGER figure.
	if within <= httpserver.ResponseDeadline {
		t.Errorf("the handler had %s left, which is inside the server's %s — a model route bounded "+
			"at the server's figure is cut by the middleware that exists to let it finish",
			within, httpserver.ResponseDeadline)
	}
}

// deadlineWriter is a recorder that accepts a write deadline, which
// httptest.NewRecorder alone does not. Unwrap is what http.NewResponseController
// reaches the recorder through; without it the middleware takes its refusal path
// and the test measures that instead.
type deadlineWriter struct {
	*httptest.ResponseRecorder
}

func (d *deadlineWriter) Unwrap() http.ResponseWriter { return d.ResponseRecorder }

func (*deadlineWriter) SetWriteDeadline(time.Time) error { return nil }

// And a chain that cannot extend the deadline fails LOUDLY rather than serving
// a response that dies mid-write, which is the symptom this exists to remove.
func TestAChainThatCannotExtendSaysSo(t *testing.T) {
	handler := boundByResponseDeadline(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
