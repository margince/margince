// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
)

// The anonymous capabilities probe: what the login screen is allowed to know
// before anybody has signed in, and the one rate budget every read behind it
// spends.
//
// Its own file rather than sitting with the session handlers, because it is the
// only route in this package answered for a caller with no principal at all,
// and it is the one that decides what a reader is shown before they can prove
// anything.

// WithFirstRunFn injects the answer to "has this installation finished
// setup", read fresh on every request the same way WithOIDCProvidersEnabledFn
// is: a settings change (finishing the model binding, say) must be visible to
// the next login-screen paint without a restart.
//
// The composition root derives it from installationSetupHandlers rather than
// this module recomputing it: identity has no import of the ai or capture
// modules the underlying steps read, and must not gain one just to answer
// this one boolean.
func (h Handlers) WithFirstRunFn(fn func(context.Context) (bool, error)) Handlers {
	h.firstRunFn = fn
	return h
}

// resolveFirstRun answers AuthCapabilities.first_run. A nil resolver, or one
// that fails, degrades to false — the ordinary sign-in screen — rather than
// failing the whole capabilities probe: this response is what the login UI
// renders from, and a momentary read failure must cost a reader the welcome
// screen, never the ability to sign in at all.
func (h Handlers) resolveFirstRun(ctx context.Context) bool {
	if h.firstRunFn == nil {
		return false
	}
	firstRun, err := h.firstRunFn(ctx)
	if err != nil {
		slog.WarnContext(ctx, "the first-run signal could not be read; this login screen renders as an ordinary sign-in",
			"reason", err)
		return false
	}
	return firstRun
}

// GetAuthCapabilities implements (GET /auth/capabilities): the anonymous
// probe the login UI renders from (A107/ADR-0061). It reports exactly the
// operational methods — a disabled provider button or a dead
// "Forgot password?" link is a misleading affordance — and discloses
// nothing beyond what the login UI needs.
//
// The release version is part of what the login UI needs, because the web tier
// cannot compare without it (compose/releaseversion.go carries why there is
// anything to compare). An unstamped build reports NOTHING rather than an empty
// string: absence is what the contract gives a client permission to ignore, and
// an empty value would be a version the client then has to know is not one.
func (h Handlers) GetAuthCapabilities(w http.ResponseWriter, r *http.Request) {
	caps := crmcontracts.AuthCapabilities{
		Password:      true,
		PasswordReset: h.canSendPasswordLink(),
	}
	if buildinfo.Comparable(buildinfo.ReleaseVersion) {
		// A local copy because the contract field is optional and therefore a
		// pointer; absence is the answer for an unstamped build.
		release := buildinfo.ReleaseVersion
		caps.ReleaseVersion = &release
	}
	caps.OidcProviders = make([]struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}, 0)
	// Spent ONCE and shared by every read this probe does against the
	// database (the provider policy below, the first-run signal beside it):
	// a flood must cost an attacker the same one budget rather than each
	// added read buying itself a second, uncapped one.
	allowed := h.capabilitiesAllowed(r)
	if h.oidcProvidersEnabledFn != nil && allowed {
		// A failed read reports NO providers rather than refusing the request.
		// This endpoint is what the login screen renders from, so an error here
		// must degrade to the method every installation always has — password —
		// instead of leaving a reader with no way in at all. The routes
		// themselves fail closed separately (StartOidcSignIn), so reporting a
		// short list can never admit a sign-in the policy would refuse.
		enabled, err := h.oidcProvidersEnabledFn(r.Context())
		if err != nil {
			slog.WarnContext(r.Context(), "the enabled sign-in providers could not be read; this login screen offers password only",
				"reason", err)
		}
		for _, p := range enabled {
			if !h.oidcProviderOffered(r.Context(), p.Key) {
				continue
			}
			caps.OidcProviders = append(caps.OidcProviders, struct {
				Key   string `json:"key"`
				Label string `json:"label"`
			}{Key: p.Key, Label: p.Label})
		}
	}
	if allowed {
		caps.FirstRun = h.resolveFirstRun(r.Context())
	}
	// NO-STORE, and the release version is what makes it mandatory rather than
	// tidy. This response is not per-principal, so a shared cache leaks nothing —
	// but the SPA refuses to render at all when the release it reads here differs
	// from its own, so one stale copy held by any cache on this origin turns a
	// healthy installation into the mixed-release screen for every reader served
	// from it, and reloading cannot clear it. A validator-less 200 GET is exactly
	// what an intermediary assigns heuristic freshness to.
	w.Header().Set("Cache-Control", "no-store")
	httperr.WriteJSON(w, http.StatusOK, caps)
}

// oidcProviderOffered reports whether a provider the policy allows has a client
// to run on right now. The policy says what the admin permits; the source says
// what the installation can actually do, and a button for a provider whose
// client is not there yet is the dead button this probe exists to avoid.
//
// A provider with no source registered is offered on the policy's word alone,
// the same posture the unwired policy takes for the routes: a handler set
// built outside NewHandlers keeps working exactly as it did. A source that
// FAILS withholds the button and says why in the log — the login screen still
// renders, password remains, and the failure is not misread as "nothing
// configured".
func (h Handlers) oidcProviderOffered(ctx context.Context, key string) bool {
	source, ok := h.oidcProviders[key]
	if !ok {
		return true
	}
	_, available, err := source(ctx)
	if err != nil {
		slog.WarnContext(ctx, "the sign-in provider's client could not be resolved; this login screen withholds it",
			"provider", key, "reason", err)
		return false
	}
	return available
}

// capabilitiesAllowed spends this caller's capabilities budget, and reports
// whether the provider policy may be read for them.
//
// Exceeding it withholds the PROVIDER list rather than refusing the request:
// the login screen still has to render, and password is the method that always
// remains. A flood therefore costs an attacker the buttons and costs the
// installation no pool connection — which is the whole point, since this route
// is anonymous and the read behind it is a transaction.
//
// An unwired limiter allows, so a handler set built outside NewHandlers keeps
// working exactly as it did.
func (h Handlers) capabilitiesAllowed(r *http.Request) bool {
	if h.capabilitiesPerIP == nil {
		return true
	}
	return h.capabilitiesPerIP.Allow(httpserver.ClientIP(r))
}
