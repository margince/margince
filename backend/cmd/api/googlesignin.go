// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"io"

	"github.com/margince/margince/backend/internal/compose"
)

// googleSignInOptions wires Google sign-in (login) on the SAME client Gmail
// capture uses — the installation's stored Google app, else the
// MARGINCE_GMAIL_CLIENT_ID/SECRET pair — with no new env vars. The pair is
// optional: what the deployment must supply is the state key and the URLs,
// and with those the routes mount and wait for whichever client arrives.
// Absent those, oidc_providers stays empty and /auth/oidc/google/* 404s
// (identity's own per-provider lookup handles that; nothing here needs to
// 501 a route explicitly).
//
// The three URLs and the boot-time check on their bases are signinurls.go's:
// they are properties of the DEPLOYMENT rather than of Google, and Microsoft's
// flow needs the identical answers. What stays here is what is actually
// Google's — which credential pair, and what the boot log calls it.
//
// (connectorHandlers.callbackURL, connectors.go, builds its own redirect_uri
// from the same flag with no such check — a gap this file does not widen but
// does not close either.)
func googleSignInOptions(cfg apiConfig, stdout io.Writer) ([]compose.Option, error) {
	redirectBase, postLogin, failure := signInURLs(cfg)
	ssoCfg := compose.GoogleSignInConfig{
		ClientID:     cfg.gmailClientID,
		ClientSecret: cfg.gmailClientSecret,
		StateKey:     cfg.connectorStateKey,
		RedirectBase: redirectBase,
		PostLoginURL: postLogin,
		FailureURL:   failure,
	}
	if ssoCfg.Enabled() {
		if err := validateSignInBases(cfg, "google"); err != nil {
			return nil, err
		}
	}
	switch {
	case ssoCfg.Enabled() && ssoCfg.HasEnvClient():
		_, _ = fmt.Fprintln(stdout, "api google sign-in enabled (/auth/oidc/google/*)")
	case ssoCfg.Enabled():
		_, _ = fmt.Fprintln(stdout, "api google sign-in mounted (/auth/oidc/google/*); offered once a Google app is stored under Settings, or MARGINCE_GMAIL_CLIENT_ID/SECRET are set")
	case cfg.gmailClientID != "":
		_, _ = fmt.Fprintf(stdout, "api google sign-in configured but INCOMPLETE — missing %v; oidc_providers stays empty\n", ssoCfg.MissingFields())
	}
	return []compose.Option{compose.WithGoogleSignIn(ssoCfg)}, nil
}
