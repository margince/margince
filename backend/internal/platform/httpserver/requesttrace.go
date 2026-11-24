// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestTrace is what the layers inside AccessLog tell it about the request.
// AccessLog sits outside auth and outside chi, so it cannot read the matched
// route or whether a session answered. NoteRoute fills this in once both are
// known. A request that never reaches chi leaves it empty, so its log line has
// no route and its response has no Server-Timing.
type requestTrace struct {
	route    string
	signedIn bool
}

type requestTraceKey struct{}

// NoteRoute records the matched route pattern and the signed-in state on the
// request's trace. It is applied as operation middleware, after chi has matched
// and the session middleware has answered.
//
// Server-Timing follows signedIn. An exact server time on a public or sign-in
// route would let a caller measure what the route hides. Login, for one, runs
// a decoy hash for an unknown user.
func NoteRoute(routeOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if trace, ok := r.Context().Value(requestTraceKey{}).(*requestTrace); ok {
				trace.route = routeOf(r)
				_, trace.signedIn = principal.Actor(r.Context())
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TraceContract gives every request under BaseURL a correlation_id and an
// access line. It wraps the whole server handler, outside the mux and the
// panic guard. So a ServeMux redirect, a refusal from a layer in front of the
// routes and the guard's own 500 are answered through the recorder too. Other
// mounts bring their own Correlate and AccessLog.
func TraceContract(log *slog.Logger, next http.Handler) http.Handler {
	traced := Correlate(AccessLog(log, next))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == BaseURL || strings.HasPrefix(r.URL.Path, BaseURL+"/") {
			traced.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serverTiming is the whole Server-Timing value: one total, in milliseconds,
// and nothing about queries or handlers. Those stay in the access log.
func serverTiming(took time.Duration) string {
	return fmt.Sprintf("total;dur=%.1f", float64(took.Microseconds())/1000)
}
