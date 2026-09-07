// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every answer on a credential-bearing public path is uncacheable, whichever
// layer produces it.
//
// A path that carries a credential in a URL segment answers from a bearer token
// sitting in somebody's mailbox for weeks. A shared cache holding any of those
// answers — the record itself, a consent state, or merely the fact that a token
// is expired — hands the next reader through that cache somebody else's data.
//
// This runs OUTSIDE the session middleware for a reason that was a live bug: the
// per-edge middlewares set the header themselves, but identity.Handlers.Middleware
// wraps them and answers first when the installation is not bootstrapped or its
// workspace cannot be resolved. Those two answers left with no Cache-Control at
// all, and a census that only drove the routed paths could not see it. Setting
// the header above everything that can answer is what makes "every answer"
// true rather than "every answer we thought of".
//
// The prefixes come from shared/kernel/capabilitypath rather than being named
// here: that package already decides which routes carry a credential, for the
// neighbouring reason that such a path must not be written to a log. One
// decision, two consequences.

import (
	"net/http"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
)

func noStoreOnCredentialPaths(next http.Handler) http.Handler {
	prefixes := capabilitypath.CredentialPrefixes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, prefix := range prefixes {
			if strings.HasPrefix(r.URL.Path, prefix) {
				w.Header().Set("Cache-Control", "no-store")
				break
			}
		}
		next.ServeHTTP(w, r)
	})
}
