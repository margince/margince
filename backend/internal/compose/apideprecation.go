// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "net/http"

func announceDeprecatedOperation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setDeprecationHeaders(w, r)
		next.ServeHTTP(w, r)
	})
}

func setDeprecationHeaders(w http.ResponseWriter, r *http.Request) {
	for name, value := range deprecationTable[r.Method+" "+chiRoutePattern(r)] {
		//nolint:goconst // the generated deprecationTable repeats the header name; this is its one reader
		if name == "Link" {
			w.Header().Add(name, value)
		} else {
			w.Header().Set(name, value)
		}
	}
}
