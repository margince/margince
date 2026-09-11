// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The anonymous preference-center edge (B-E11.32): /v1/public/preferences/*
// carries neither session nor workspace header, so this middleware —
// composed like the public-booking edge — resolves the token to its tenant,
// throttles the unauthenticated surface, and binds the workspace plus a
// system principal confined to the preference endpoints. Everything
// downstream (RBAC-gated consent store, audit attribution as
// actor_type=system) then works unchanged.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const publicPreferencesPrefix = "/v1/public/preferences/"

// publicPreferenceLimiters mirror the booking edge: rate limiting is the
// only brake on an anonymous surface. Per-IP covers scripted scraping;
// per-token covers a flood aimed at one recipient's consent state. The
// one-click POST is the sensitive verb, so the per-token brake applies to
// mutations.
type publicPreferenceLimiters struct {
	perIP    *ratelimit.Limiter
	perToken *ratelimit.Limiter
}

func newPublicPreferenceLimiters() publicPreferenceLimiters {
	return publicPreferenceLimiters{
		perIP:    ratelimit.New(60, time.Minute),
		perToken: ratelimit.New(20, time.Minute),
	}
}

func publicPreferences(store *consent.Store, limits publicPreferenceLimiters) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, publicPreferencesPrefix) {
				next.ServeHTTP(w, r)
				return
			}
			// Cache-Control: no-store is NOT set here — see the same note
			// on the confirm edge. One writer, above everything that can
			// answer on these prefixes.
			token := strings.SplitN(strings.TrimPrefix(r.URL.Path, publicPreferencesPrefix), "/", 2)[0]
			if token == "" {
				httperr.Write(w, r, apperrors.ErrNotFound)
				return
			}
			if !limits.perIP.Allow(httpserver.ClientIP(r)) {
				httperr.Write(w, r, apperrors.ErrBudgetExceeded)
				return
			}
			if r.Method != http.MethodGet && !limits.perToken.Allow(token) {
				httperr.Write(w, r, apperrors.ErrBudgetExceeded)
				return
			}

			// Resolved for its refusal, not its answer: the handlers resolve
			// the token again for the contact it names, while this gate exists
			// to turn an unknown, revoked or expired token away before any of
			// them run. Unknown and revoked read identically as absent — the
			// surface never becomes a consent-state oracle.
			//
			// EITHER FAMILY OPENS THIS EDGE, and only the one-click POST can
			// act on the weaker of the two. A withdrawal credential resolves
			// here so that press reaches its handler; every other route on this
			// prefix reads or writes a consent state and refuses it again for
			// itself, because a withdrawal credential carries no authority to
			// see a purpose list, let alone grant one.
			if err := resolvesOnThisEdge(r.Context(), store, token); err != nil {
				httperr.Write(w, r, err)
				return
			}

			// The workspace is already bound: the identity middleware binds
			// the installation's into every request context, public paths
			// included, before this runs.
			ctx := principal.WithActor(r.Context(), principal.Principal{
				Type: principal.PrincipalSystem,
				ID:   "system:public_preferences",
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolvesOnThisEdge admits a token of either family, so the one-click POST
// can carry the long-lived credential while the preference centre keeps
// requiring the short-lived one.
//
// It answers the SAME error for both misses, which is what keeps the edge from
// reporting which family a probed string belonged to.
func resolvesOnThisEdge(ctx context.Context, store *consent.Store, token string) error {
	if _, err := store.ResolvePreferenceToken(ctx, token); err == nil {
		return nil
	}
	if _, err := store.ResolveWithdrawalToken(ctx, token); err != nil {
		return apperrors.ErrNotFound
	}
	return nil
}
