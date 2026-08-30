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
func googleSignInOptions(cfg apiConfig, stdout io.Writer) []compose.Option {
	ssoCfg := compose.GoogleSignInConfig{
		ClientID:     cfg.gmailClientID,
		ClientSecret: cfg.gmailClientSecret,
		StateKey:     cfg.connectorStateKey,
		RedirectBase: cfg.publicBaseURL,
		PostLoginURL: "/",
		FailureURL:   "/#/login?oidc=failed",
	}
	switch {
	case ssoCfg.Enabled():
		_, _ = fmt.Fprintln(stdout, "api google sign-in enabled (/auth/oidc/google/*)")
	case cfg.gmailClientID != "":
		_, _ = fmt.Fprintf(stdout, "api google sign-in configured but INCOMPLETE — missing %v; oidc_providers stays empty\n", ssoCfg.MissingFields())
	}
	return []compose.Option{compose.WithGoogleSignIn(ssoCfg)}
}
