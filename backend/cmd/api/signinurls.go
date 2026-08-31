// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import "fmt"

// The three URLs every federated sign-in flow travels through, and the check
// that they are safe to build into an outbound redirect.
//
// Shared because they are a property of the DEPLOYMENT, not of the vendor:
// where the api answers, where the SPA lands, and where a refusal goes are the
// same three answers whether the round trip went to Google or to Microsoft.
// Only the credentials and the vendor's own knobs differ, and those stay in
// each provider's own file.

// signInURLs resolves the deployment's sign-in URLs.
//
// redirectBase (where the PROVIDER sends the browser back) and postLogin /
// failure (where the callback then sends it) read DIFFERENT bases on purpose:
// --api-base-url is set only where the api and the SPA are different origins,
// and the provider's redirect_uri must reach the api while the human-facing
// landing must reach the SPA.
func signInURLs(cfg apiConfig) (redirectBase, postLogin, failure string) {
	redirectBase = cfg.apiBaseURL
	if redirectBase == "" {
		redirectBase = cfg.publicBaseURL
	}
	base := trimTrailingSlash(cfg.publicBaseURL)
	return redirectBase, base + "/", base + "/#/login?oidc=failed"
}

// validateSignInBases refuses at BOOT a base URL this deployment would
// otherwise bake into a redirect_uri and send to the provider.
//
// A boot-time refusal is the only place that catches a malformed or
// credential-bearing base before it reaches the provider's own server logs.
// --api-base-url gets the same check --public-base-url gets, when set, for
// exactly that reason.
//
// provider names the flow in the error so an operator reading a boot failure
// knows which one refused — the two are configured independently and can fail
// for different reasons.
func validateSignInBases(cfg apiConfig, provider string) error {
	if err := validatePublicBaseURL(cfg.publicBaseURL); err != nil {
		return fmt.Errorf("api: %s sign-in: %w", provider, err)
	}
	if cfg.apiBaseURL != "" {
		if err := validateBareOrigin("--api-base-url", cfg.apiBaseURL); err != nil {
			return fmt.Errorf("api: %s sign-in: %w", provider, err)
		}
	}
	return nil
}

// trimTrailingSlash normalises a base before a path is appended, so the result
// never carries a doubled separator.
func trimTrailingSlash(base string) string {
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base
}
