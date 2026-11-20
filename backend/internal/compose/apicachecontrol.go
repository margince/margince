// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "net/http"

// apiDefaultCache is the Cache-Control of every /v1 answer whose handler states
// none. Each answer is one signed-in reader's view of private data, so no
// browser or shared cache may keep it.
const apiDefaultCache = "private, no-store"

// privateUnlessStated sets apiDefaultCache before the handler runs, so a handler
// that knows better (an ETag re-read, an avatar) sets its own value over it. A
// value an outer layer already set, such as a credential path's no-store, stays.
// It wraps the whole /v1 mount, so the session middleware's own refusals and the
// extension routes carry it too.
func privateUnlessStated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if w.Header().Get("Cache-Control") == "" {
			w.Header().Set("Cache-Control", apiDefaultCache)
		}
		next.ServeHTTP(w, r)
	})
}
