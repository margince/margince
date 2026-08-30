// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"io"

	"github.com/margince/margince/backend/internal/compose"
)

// googleSignInOptions wires Google sign-in (login) using the SAME
// MARGINCE_GMAIL_CLIENT_ID/SECRET pair Gmail capture uses — no new env
// vars. Self-gates like gmailOptions: absent/incomplete config,
// oidc_providers stays empty and /auth/oidc/google/* 404s (identity's own
// per-provider lookup handles that; nothing here needs to 501 a route
// explicitly).
//
// RedirectBase is validated here rather than trusted from bindInstallation's
// own --public-base-url check: that check runs only when
// mcp.connector_enabled, and an installation that runs Google sign-in
// without the MCP connector would otherwise bake a malformed or
// credential-bearing base straight into the redirect_uri this sends to
// Google — a boot-time refusal is the only place that can catch it before
// it reaches a request Google's own server logs.
func googleSignInOptions(cfg apiConfig, stdout io.Writer) ([]compose.Option, error) {
	ssoCfg := compose.GoogleSignInConfig{
		ClientID:     cfg.gmailClientID,
		ClientSecret: cfg.gmailClientSecret,
		StateKey:     cfg.connectorStateKey,
		RedirectBase: cfg.publicBaseURL,
		PostLoginURL: "/",
		FailureURL:   "/#/login?oidc=failed",
	}
	if ssoCfg.Enabled() {
		if err := validatePublicBaseURL(cfg.publicBaseURL); err != nil {
			return nil, fmt.Errorf("api: google sign-in: %w", err)
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
