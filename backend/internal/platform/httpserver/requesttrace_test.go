// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const testRoute = "GET /v1/brief"

// accessLine is the one "http request" line a request wrote.
type accessLine struct {
	CorrelationID string `json:"correlation_id"`
	Status        int    `json:"status"`
	Route         string `json:"route"`
	DBMS          *int64 `json:"db_ms"`
	PoolWaitMS    *int64 `json:"pool_wait_ms"`
	Statements    *int64 `json:"statements"`
}

// serveTraced runs inner under the production order of RecoverPanics, Correlate
// and AccessLog, with NoteRoute inside as the contract router applies it. A
// signed-in request is one whose context carries an actor when NoteRoute runs.
func serveTraced(t *testing.T, signedIn bool, route string, inner http.HandlerFunc) (*httptest.ResponseRecorder, accessLine) {
	t.Helper()
	var output strings.Builder
	logger := slog.New(WithCorrelation(slog.NewJSONHandler(&output, nil)))
	routed := NoteRoute(func(*http.Request) string { return route })(inner)
	withActor := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if signedIn {
			r = r.WithContext(principal.WithActor(r.Context(), principal.Principal{ID: "human:" + ids.NewV7().String()}))
		}
		routed.ServeHTTP(w, r)
	})
	return serveLogged(t, &output, TraceContract(logger, RecoverPanics(logger, withActor)), "/v1/brief")
}

// serveLogged serves one GET and returns the response and its access line.
func serveLogged(t *testing.T, output *strings.Builder, handler http.Handler, path string) (*httptest.ResponseRecorder, accessLine) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))

	var line accessLine
	for raw := range strings.SplitSeq(strings.TrimSpace(output.String()), "\n") {
		var parsed struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed.Msg == "http request" {
			if err := json.Unmarshal([]byte(raw), &line); err != nil {
				t.Fatal(err)
			}
		}
	}
	return response, line
}

func TestEveryAnswerNamesItsRequestAndTheAccessLineCarriesTheSameID(t *testing.T) {
	answers := map[string]http.HandlerFunc{
		"a handler that only writes": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) },
		"an error answer":            func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"a panic":                    func(http.ResponseWriter, *http.Request) { panic("boom") },
	}
	for name, inner := range answers {
		t.Run(name, func(t *testing.T) {
			response, line := serveTraced(t, true, testRoute, inner)
			id := response.Header().Get("X-Request-Id")
			if id == "" || line.CorrelationID != id {
				t.Fatalf("response X-Request-Id %q, access line correlation_id %q: want one id on both", id, line.CorrelationID)
			}
			if alias := response.Header().Get("X-Correlation-ID"); alias != id {
				t.Fatalf("deprecated X-Correlation-ID %q, want the same id %q", alias, id)
			}
			if line.Status != response.Code {
				t.Fatalf("access line status %d, client received %d", line.Status, response.Code)
			}
		})
	}
}

func TestServerTimingOnlyAnswersASignedInRequest(t *testing.T) {
	writeOnly := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) }
	withHeader := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}
	for name, handler := range map[string]http.HandlerFunc{"Write only": writeOnly, "WriteHeader": withHeader} {
		t.Run(name+", signed in", func(t *testing.T) {
			response, _ := serveTraced(t, true, testRoute, handler)
			if got := response.Header().Get("Server-Timing"); !strings.HasPrefix(got, "total;dur=") {
				t.Fatalf("Server-Timing %q, want total;dur=<ms>", got)
			}
		})
		t.Run(name+", signed out", func(t *testing.T) {
			response, _ := serveTraced(t, false, testRoute, handler)
			if got := response.Header().Get("Server-Timing"); got != "" {
				t.Fatalf("Server-Timing %q on a request no session answered", got)
			}
		})
	}
}

func TestAccessLineNamesTheRouteAndTheDatabaseCost(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	_, routed := serveTraced(t, true, testRoute, ok)
	if routed.Route != testRoute {
		t.Fatalf("route %q, want %q", routed.Route, testRoute)
	}
	if routed.DBMS == nil || routed.PoolWaitMS == nil || routed.Statements == nil {
		t.Fatalf("access line lacks db_ms, pool_wait_ms or statements: %+v", routed)
	}
	_, unrouted := serveTraced(t, true, "", ok)
	if unrouted.Route != "" {
		t.Fatalf("a request chi never matched logged route %q", unrouted.Route)
	}
}

func TestAFlushCommitsTheStatusAndServerTimingBeforeAnyBody(t *testing.T) {
	flushOnly := func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
	}
	response, line := serveTraced(t, true, testRoute, flushOnly)
	if got := response.Result().Header.Get("Server-Timing"); !strings.HasPrefix(got, "total;dur=") {
		t.Fatalf("stream Server-Timing %q, want total;dur=<ms>", got)
	}
	if line.Status != http.StatusOK {
		t.Fatalf("access line status %d, want 200", line.Status)
	}
}

func TestAPanicAfterAFlushIsLoggedWithTheStatusTheClientGot(t *testing.T) {
	_, line := serveTraced(t, true, testRoute, func(w http.ResponseWriter, _ *http.Request) {
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
		panic("after the stream began")
	})
	if line.Status != http.StatusOK {
		t.Fatalf("access line status %d, but the client already received 200", line.Status)
	}
}

func TestAPanicBeforeAnyOutputAnswers500WithServerTiming(t *testing.T) {
	response, line := serveTraced(t, true, testRoute, func(http.ResponseWriter, *http.Request) { panic("early") })
	if response.Code != http.StatusInternalServerError || line.Status != http.StatusInternalServerError {
		t.Fatalf("client got %d and the line says %d, want 500 on both", response.Code, line.Status)
	}
	if !strings.HasPrefix(response.Result().Header.Get("Server-Timing"), "total;dur=") {
		t.Fatal("the recovered 500 of a signed-in request carries no Server-Timing")
	}
}

func TestTraceContractCoversAMuxRedirectAndSkipsOtherMounts(t *testing.T) {
	var output strings.Builder
	logger := slog.New(WithCorrelation(slog.NewJSONHandler(&output, nil)))
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/brief", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("/healthz", func(http.ResponseWriter, *http.Request) {})
	handler := TraceContract(logger, mux)

	redirect, line := serveLogged(t, &output, handler, "/v1//brief")
	if redirect.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status %d, want the mux's 307", redirect.Code)
	}
	if id := redirect.Header().Get("X-Request-Id"); id == "" || id != line.CorrelationID || line.Status != redirect.Code {
		t.Fatalf("redirect X-Request-Id %q, access line %+v", id, line)
	}

	output.Reset()
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Header().Get("X-Request-Id") != "" || output.Len() != 0 {
		t.Fatal("a route outside /v1 was traced")
	}
}
