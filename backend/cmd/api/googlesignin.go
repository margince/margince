// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/margince/margince/backend/internal/compose"
)

// googleSignInOptions wires Google sign-in (login) using the SAME
// MARGINCE_GMAIL_CLIENT_ID/SECRET pair Gmail capture uses — no new env
// vars. Self-gates like gmailOptions: absent/incomplete config,
// oidc_providers stays empty and /auth/oidc/google/* 404s (identity's own
// per-provider lookup handles that; nothing here needs to 501 a route
// explicitly).
//
// RedirectBase (where Google sends the browser back) and PostLoginURL/
// FailureURL (where the callback then sends it) deliberately read DIFFERENT
// bases, the same split gmail.go's callbackURL and connectors_outcome.go's
// landingURL already make for the connector flow: --api-base-url is set only
// on a deployment where the api and the SPA are different origins (this
// repo's own per-worktree dev stack is exactly that), and Google's
// redirect_uri must reach the api while the human-facing landing must reach
// the SPA.
//
// RedirectBase is validated here rather than trusted from bindInstallation's
// own --public-base-url check: that check runs only when
// mcp.connector_enabled, and an installation that runs Google sign-in
// without the MCP connector would otherwise bake a malformed or
// credential-bearing base straight into the redirect_uri this sends to
// Google — a boot-time refusal is the only place that can catch it before
// it reaches a request Google's own server logs. --api-base-url gets the
// same validateBareOrigin check --public-base-url gets, when set, for
// exactly the same reason: this flow builds it into an outbound redirect_uri
// too. (connectorHandlers.callbackURL, connectors.go, builds its own
// redirect_uri from the same flag with no such check — a gap this file does
// not widen but does not close either; narrowing that to one shared,
// validated accessor is future work, not this feature's scope.)
func googleSignInOptions(cfg apiConfig, stdout io.Writer) ([]compose.Option, error) {
	redirectBase := cfg.apiBaseURL
	if redirectBase == "" {
		redirectBase = cfg.publicBaseURL
	}
	ssoCfg := compose.GoogleSignInConfig{
		ClientID:     cfg.gmailClientID,
		ClientSecret: cfg.gmailClientSecret,
		StateKey:     cfg.connectorStateKey,
		RedirectBase: redirectBase,
		PostLoginURL: strings.TrimRight(cfg.publicBaseURL, "/") + "/",
		FailureURL:   strings.TrimRight(cfg.publicBaseURL, "/") + "/#/login?oidc=failed",
	}
	if ssoCfg.Enabled() {
		if err := validatePublicBaseURL(cfg.publicBaseURL); err != nil {
			return nil, fmt.Errorf("api: google sign-in: %w", err)
		}
		if cfg.apiBaseURL != "" {
			if err := validateBareOrigin("--api-base-url", cfg.apiBaseURL); err != nil {
				return nil, fmt.Errorf("api: google sign-in: %w", err)
			}
		}
	}
	switch {
	case ssoCfg.Enabled():
		_, _ = fmt.Fprintln(stdout, "api google sign-in enabled (/auth/oidc/google/*)")
	case cfg.gmailClientID != "":
		_, _ = fmt.Fprintf(stdout, "api google sign-in configured but INCOMPLETE — missing %v; oidc_providers stays empty\n", ssoCfg.MissingFields())
	}
	return []compose.Option{compose.WithGoogleSignIn(ssoCfg)}, nil
}
