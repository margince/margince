// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/pkg/extension"
)

// publicInstrumentPrefix is where a unit's Instrumentation.Edge is served,
// without a session. The browser half of a unit's instrumentation reaches its
// backend here, before anyone has signed in and past a CSP that admits only
// this origin. The segment after the prefix is the unit's name, not a
// credential.
const publicInstrumentPrefix = "/v1/public/instrument/"

// instrumentEdgeBodyLimit caps one request to a unit's edge.
const instrumentEdgeBodyLimit = 1 << 20

func newPublicInstrumentLimiter() *ratelimit.Limiter {
	return ratelimit.New("public-instrument/per-ip", ratelimit.FailClosed, 600, time.Minute)
}

func publicInstrument(exts []extension.Extension, perIP *ratelimit.Limiter) func(http.Handler) http.Handler {
	edges := make(map[string]http.Handler)
	for _, e := range exts {
		if e.Instrumentation != nil && e.Instrumentation.Edge != nil {
			unit := string(e.Name)
			edges[unit] = http.StripPrefix(publicInstrumentPrefix+unit, e.Instrumentation.Edge)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, publicInstrumentPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			unit, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, publicInstrumentPrefix), "/")
			edge, ok := edges[unit]
			if !ok {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodPost {
				w.Header().Set("Allow", "GET, POST")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if !perIP.Allow(httpserver.ClientIP(r)) {
				httperr.Write(w, r, apperrors.ErrBudgetExceeded)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, instrumentEdgeBodyLimit)
			edge.ServeHTTP(w, r)
		})
	}
}
