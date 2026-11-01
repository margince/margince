// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// forgetHomeOnWrite ends the writer's cached Home answers when a write they
// made succeeds, before its status is sent. A client that refetches on the
// answer must not read the day as it was. Many of a reader's own writes emit no
// event and no audit row, so the door they all pass is where this is asked.
func forgetHomeOnWrite(cache *homecache.Cache, log *slog.Logger) crmcontracts.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writers := writersOf(r)
			if cache == nil || len(writers) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			//nolint:contextcheck // WriteHeader has net/http's signature, and the request's context is the one this write ran under.
			next.ServeHTTP(&forgettingWriter{ResponseWriter: w, forget: func() {
				// The write has committed and its answer is the caller's; a
				// failure here leaves their Home behind by at most homecache.Life.
				if err := cache.Forget(r.Context(), writers); err != nil {
					log.Error("ending a writer's cached Home", "err", err)
				}
			}}, r)
		})
	}
}

// writersOf names the readers a mutating request writes for: the human, or the
// human an agent acts for. A read writes for nobody.
func writersOf(r *http.Request) []ids.UUID {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return nil
	}
	actor, ok := principal.Actor(r.Context())
	if !ok {
		return nil
	}
	var out []ids.UUID
	for _, id := range []ids.UUID{actor.UserID, actor.OnBehalfOf} {
		if !id.IsZero() {
			out = append(out, id)
		}
	}
	return out
}

// forgettingWriter runs forget once, as a successful status is first written.
type forgettingWriter struct {
	http.ResponseWriter
	forget  func()
	settled bool
}

func (f *forgettingWriter) WriteHeader(status int) {
	if !f.settled {
		f.settled = true
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			f.forget()
		}
	}
	f.ResponseWriter.WriteHeader(status)
}

// Write without a header is the 200 net/http sends for it.
func (f *forgettingWriter) Write(body []byte) (int, error) {
	if !f.settled {
		f.WriteHeader(http.StatusOK)
	}
	return f.ResponseWriter.Write(body)
}

// Unwrap lets http.NewResponseController reach the connection through this layer.
func (f *forgettingWriter) Unwrap() http.ResponseWriter { return f.ResponseWriter }
