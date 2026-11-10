// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func readerDisplayLocale(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			actor, ok := principal.Actor(ctx)
			if ok && actor.Type == principal.PrincipalHuman {
				locale := r.Header.Get("X-Margince-Display-Locale")
				if !textlang.Known(locale) {
					locale = identity.ReaderLanguageForPrompt(ctx, pool)
				}
				ctx = systemtext.WithLocale(ctx, textlang.Lang(locale))
				w.Header().Add("Vary", "X-Margince-Display-Locale")
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
