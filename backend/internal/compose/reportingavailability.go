// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// WithReportingEnabled keeps reporting routes and the worker behind the same rollout flag.
func WithReportingEnabled(enabled bool) Option {
	return func(s *Server, _ *pgxpool.Pool) { s.reportingEnabled = enabled }
}

func reportingAvailability(enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled && reportingRoute(r.URL.Path) {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func reportingRoute(path string) bool {
	path = strings.TrimSuffix(strings.TrimPrefix(path, "/v1"), ".csv")
	if path == "/analytics/reports/render" {
		return false
	}
	for _, resource := range []string{"metrics", "evaluate", "evidence", reportingReports, "targets", "framework", "schedules", reportingEditions, "executions"} {
		prefix := "/analytics/" + resource
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
