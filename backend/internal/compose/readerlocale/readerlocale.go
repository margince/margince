// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package readerlocale picks the language a request is answered in: a signed-in
// human's own, or what an anonymous reader's browser asks for.
package readerlocale

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/language"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Middleware binds a signed-in human's display language to the request.
func Middleware(pool *pgxpool.Pool) func(http.Handler) http.Handler {
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

// Anonymous is the language of a reader with no account behind the request.
func Anonymous(r *http.Request) textlang.Lang {
	if explicit := r.URL.Query().Get("locale"); textlang.Known(explicit) {
		return textlang.Lang(explicit)
	}
	if explicit := r.Header.Get("X-Margince-Display-Locale"); textlang.Known(explicit) {
		return textlang.Lang(explicit)
	}
	header := r.Header.Get("Accept-Language")
	if len(header) > 4096 {
		return textlang.English
	}
	preferred, _, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return textlang.English
	}
	supported := make([]language.Tag, len(textlang.Shipped))
	for i, locale := range textlang.Shipped {
		supported[i] = language.Make(string(locale))
	}
	_, index, _ := language.NewMatcher(supported).Match(preferred...)
	return textlang.Shipped[index]
}
