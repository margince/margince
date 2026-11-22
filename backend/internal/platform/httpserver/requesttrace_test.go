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
	handler := RecoverPanics(logger, Correlate(AccessLog(logger, withActor)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/brief", nil))

	var line accessLine
	for _, raw := range strings.Split(strings.TrimSpace(output.String()), "\n") {
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
