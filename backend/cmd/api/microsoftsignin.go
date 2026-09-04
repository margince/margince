// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"io"

	"github.com/margince/margince/backend/internal/compose"
)

// microsoftSignInOptions wires Microsoft sign-in (login) on the SAME
// MARGINCE_GRAPH_CLIENT_ID/SECRET pair Graph capture uses — no new credential,
// the economy googleSignInOptions makes with the Gmail pair. Self-gates the same
// way: absent or incomplete config, the provider never joins oidc_providers and
// /auth/oidc/microsoft/* 404s.
//
// The TENANT is the one thing this flow does not inherit. Capture's authority
// defaults to `common` on purpose — a rep authorizes their own mailbox and the
// credential reaches nothing else — while a sign-in matches the token's address
// to a member of this installation, where a tenant nobody vetted is worth
// nothing (compose/microsoftsignin.go carries the full reasoning). So sign-in
// reads its own --microsoft-signin-tenant, which falls back to --graph-tenant
// when that already names one directory rather than an authority alias. An
// installation running `common` capture therefore gets no Microsoft sign-in
// until it says which directories its people are in, and the boot log says so.
//
// DIRECTORIES, plural, because a flag holding exactly one made "any Entra
// tenant" mean "no sign-in at all". Each entry is a directory whose
// administrators this installation vouches for, which is a decision somebody
// can make; an alias is not, and stays refused.
//
// RedirectBase (where Microsoft sends the browser back) and PostLoginURL/
// FailureURL (where the callback then sends it) read DIFFERENT bases, the same
// split googleSignInOptions makes: --api-base-url is set only where the api and
// the SPA are different origins, and Microsoft's redirect_uri must reach the api
// while the human-facing landing must reach the SPA.
func microsoftSignInOptions(cfg apiConfig, stdout io.Writer) ([]compose.Option, error) {
	redirectBase, postLogin, failure := signInURLs(cfg)
	ssoCfg := compose.MicrosoftSignInConfig{
		ClientID:     cfg.graphClientID,
		ClientSecret: cfg.graphClientSecret,
		Tenant:       microsoftSignInTenant(cfg),
		StateKey:     cfg.connectorStateKey,
		RedirectBase: redirectBase,
		PostLoginURL: postLogin,
		FailureURL:   failure,
	}
	if ssoCfg.Enabled() {
		if err := validateSignInBases(cfg, "microsoft"); err != nil {
			return nil, err
		}
	}
	switch {
	case ssoCfg.Enabled() && ssoCfg.HasEnvClient():
		_, _ = fmt.Fprintf(stdout, "api microsoft sign-in enabled (/auth/oidc/microsoft/*, directories %s)\n", ssoCfg.Tenant)
	case ssoCfg.Enabled() && cfg.graphClientID != "":
		// The pair is there and cannot sign anyone in: nothing names a directory
		// its tokens may come from. Said out loud, because the operator who set
		// the pair expects the button, and a stored app pinned to a directory
		// would put it there without them knowing why.
		_, _ = fmt.Fprintln(stdout, "api microsoft sign-in mounted (/auth/oidc/microsoft/*); the environment's client is offered once --microsoft-signin-tenant names a directory, and a Microsoft app stored under Settings is offered on the directory it is pinned to")
	case ssoCfg.Enabled():
		_, _ = fmt.Fprintln(stdout, "api microsoft sign-in mounted (/auth/oidc/microsoft/*); offered once a Microsoft app pinned to a directory is stored under Settings, or MARGINCE_GRAPH_CLIENT_ID/SECRET and --microsoft-signin-tenant are set")
	case cfg.graphClientID != "":
		_, _ = fmt.Fprintf(stdout, "api microsoft sign-in configured but INCOMPLETE — missing %v; the provider stays off\n", ssoCfg.MissingFields())
	}
	if ssoCfg.Enabled() {
		_, _ = fmt.Fprintf(stdout, "api microsoft sign-in redirect URI to register on the Entra app: %s\n",
			compose.MicrosoftSignInRedirectURI(redirectBase))
	}
	return []compose.Option{compose.WithMicrosoftSignIn(ssoCfg)}, nil
}

// microsoftSignInTenant resolves the directories sign-in accepts: its own flag,
// or the capture connector's when that already names one.
//
// The fallback is deliberately one-way. Inheriting a real directory id saves an
// operator from configuring the same GUID twice, while inheriting `common` (or
// any other authority alias) would silently widen a login to every Entra tenant
// on earth. compose.MicrosoftSignInConfig refuses the alias either way; passing
// it through rather than blanking it is what lets MissingFields NAME the reason.
func microsoftSignInTenant(cfg apiConfig) string {
	if cfg.microsoftSignInTenant != "" {
		return cfg.microsoftSignInTenant
	}
	return cfg.graphTenant
}
