// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forgetHomeOnWrite ends the writer's cached Home after a write that succeeds or
// conflicts (homecache.AfterWrite says how). Many writes emit no event or audit row.
func forgetHomeOnWrite(cache *homecache.Cache, log *slog.Logger) crmcontracts.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writers := writersOf(r)
			if cache == nil || len(writers) == 0 || !mayChangeHome(r) {
				next.ServeHTTP(w, r)
				return
			}
			//nolint:contextcheck // WriteHeader has net/http's signature, and the request's context is the one this write ran under.
			next.ServeHTTP(homecache.NewForgettingWriter(w, func(status int) {
				// The write has committed and its answer is the caller's; a
				// failure here leaves their Home behind by at most homecache.Life.
				if err := cache.AfterWrite(r, writers, status); err != nil {
					log.Error("ending a writer's cached Home", "err", err)
				}
			}), r)
		})
	}
}

// homeUnchangedBy lists the writes that cannot change what Home shows, so they
// do not mark it behind. Each is reviewed; a write not listed marks Home.
var homeUnchangedBy = map[string]string{
	http.MethodPost + " /v1/companies/{id}/view-ack": "moves only the since-last-visit baseline of one company page",
	http.MethodPost + " /v1/contacts/{id}/view-ack":  "moves only the since-last-visit baseline of one contact page",
	http.MethodPut + " /v1/mail-drafts":              "autosaves an unsent message that no Home row reads",
}

// mayChangeHome is false for a route with the read scope, a POST that only
// asks a question. It is also false for the writes in homeUnchangedBy.
func mayChangeHome(r *http.Request) bool {
	if routePolicy(r).Scope == scopeRead {
		return false
	}
	route := chi.RouteContext(r.Context())
	if route == nil {
		return true
	}
	_, unchanged := homeUnchangedBy[r.Method+" "+route.RoutePattern()]
	return !unchanged
}

// writersOf names the readers a mutating request writes for: the human, or the
// human an agent acts for. A read writes for nobody.
func writersOf(r *http.Request) []ids.UUID {
	// The agent gate's own reading of which requests write, so the two never
	// disagree about one route.
	if !mutatingMethod(r.Method) {
		return nil
	}
	actor, ok := principal.Actor(r.Context())
	if !ok {
		return nil
	}
	// An agent's UserID is often the human it acts for, so the two can be one.
	var out []ids.UUID
	for _, id := range []ids.UUID{actor.UserID, actor.OnBehalfOf} {
		if !id.IsZero() && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
