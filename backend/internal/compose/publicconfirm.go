// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The anonymous confirm-details edge: /v1/public/confirm/* carries neither
// session nor workspace header, so this middleware — composed exactly like the
// preference edge beside it — throttles the unauthenticated surface and binds a
// system principal confined to these endpoints.
//
// It deliberately does NOT pre-resolve the token the way the preference edge
// does. That one resolves for its refusal, which is free because resolving a
// preference token is a pure read. Resolving a confirm token STAMPS opened_at,
// so a pre-flight here would record an opening for every request including the
// POST — and the handler resolves anyway. One resolution per request, in the
// handler, keeps that evidence honest.

import (
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const publicConfirmPrefix = "/v1/public/confirm/"

// publicConfirmLimiters mirror the preference edge: rate limiting is the only
// brake on an anonymous surface. Per-IP covers scripted scraping of tokens;
// per-token covers a flood aimed at one recipient. Both verbs count against the
// per-token brake here, unlike the preference edge — a GET on this surface
// discloses somebody's record, where a GET there lists purpose switches.
type publicConfirmLimiters struct {
	perIP    *ratelimit.Limiter
	perToken *ratelimit.Limiter
}

func newPublicConfirmLimiters() publicConfirmLimiters {
	return publicConfirmLimiters{
		perIP:    ratelimit.New(60, time.Minute),
		perToken: ratelimit.New(20, time.Minute),
	}
}

func publicConfirm(limits publicConfirmLimiters) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, publicConfirmPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			// Set before any refusal below, for the preference edge's reason
			// and with more at stake: every response on this prefix is derived
			// from a bearer token that lives in somebody's mailbox, and a GET
			// here discloses that person's record rather than a list of
			// switches. A shared cache holding any of them is a leak, so the
			// rate-limited and not-found answers are as uncacheable as the
			// successful one.
			w.Header().Set("Cache-Control", "no-store")
			token := strings.SplitN(strings.TrimPrefix(r.URL.Path, publicConfirmPrefix), "/", 2)[0]
			if token == "" {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			if !limits.perIP.Allow(httpserver.ClientIP(r)) {
				httperr.Write(w, r, apperrors.ErrBudgetExceeded)
				return
			}
			if !limits.perToken.Allow(token) {
				httperr.Write(w, r, apperrors.ErrBudgetExceeded)
				return
			}

			// The workspace is already bound: the identity middleware binds the
			// installation's into every request context, public paths included,
			// before this runs.
			ctx := principal.WithActor(r.Context(), principal.Principal{
				Type: principal.PrincipalSystem,
				ID:   "system:public_confirm",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
