// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// AbsentPreference is the RFC 7240 preference with which a client asks for 204
// instead of 404 when a read finds nothing yet.
const AbsentPreference = "absent=204"

type absentPreferredKey struct{}

// AbsentPreferenceFor serves the absent preference on the operations declares
// names. Each of their answers varies on Prefer, so a cache never gives one
// client the shape the other asked for.
func AbsentPreferenceFor(declares func(*http.Request) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !declares(r) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Prefer")
			if PrefersAbsent(r.Header.Values("Prefer")) {
				r = r.WithContext(context.WithValue(r.Context(), absentPreferredKey{}, true))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PrefersAbsent reports whether Prefer header values carry absent=204. Tokens
// compare without case, a value may be quoted, and every other preference and
// parameter is ignored, as RFC 7240 asks of a server.
func PrefersAbsent(headers []string) bool {
	for _, header := range headers {
		for _, preference := range splitUnquoted(header, ',') {
			token, value, _ := strings.Cut(splitUnquoted(preference, ';')[0], "=")
			if !strings.EqualFold(strings.TrimSpace(token), "absent") {
				continue
			}
			if strings.Trim(strings.TrimSpace(value), `"`) == "204" {
				return true
			}
		}
	}
	return false
}

// answeredAbsent writes the 204 a request asked for in place of a nothing-yet 404.
func answeredAbsent(w http.ResponseWriter, r *http.Request, err error) bool {
	preferred, _ := r.Context().Value(absentPreferredKey{}).(bool)
	if !preferred || !errors.Is(err, apperrors.ErrNothingYet) {
		return false
	}
	w.Header().Set("Preference-Applied", AbsentPreference)
	w.WriteHeader(http.StatusNoContent)
	return true
}
