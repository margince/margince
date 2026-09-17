// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"fmt"
	"log/slog"
)

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

// checkALoginMethodRemains refuses at boot the one deployment nobody could sign
// into: the password method turned off with no federated provider wired to
// replace it.
//
// deployconfig cannot ask this. Which providers are mounted is decided by the
// credentials and URLs this role composes rather than by that document, so the
// file that holds the switch can only say what it was told — and a validation
// that guesses would either refuse a legitimate IdP-only installation or admit
// one with no door at all.
//
// The surviving posture is SAID OUT LOUD rather than merely allowed, for the
// reason the data-reset line is: an operator who closed the password door
// should learn which door is left at the moment the api starts, not from a
// login screen that offers nothing.
func checkALoginMethodRemains(cfg apiConfig, passwordEnabled bool, logger *slog.Logger) error {
	if passwordEnabled {
		return nil
	}
	mounted := federatedSignInProviders(cfg)
	if len(mounted) == 0 {
		return errors.New("api: auth.password.enabled=false and this deployment mounts no federated sign-in provider — nobody could sign in. Configure Google or Microsoft sign-in, or leave the password method enabled")
	}
	logger.Info("password sign-in disabled", "federated", mounted)
	return nil
}

// federatedSignInProviders names the providers this deployment MOUNTS — the
// ones whose routes exist and will run a flow for whichever OAuth client
// arrives, from the environment or from the app an admin stores under Settings.
//
// Mounted is the honest bar and a weaker one than "somebody can sign in today":
// a mounted provider with no client yet offers no button, by design, because
// that is what lets a first-run admin configure sign-in without a restart. So
// this answers "did the deployment wire a second door", which is the question
// the password switch needs, and not "is that door unlocked right now", which
// only the installation's own rows can answer.
func federatedSignInProviders(cfg apiConfig) []string {
	var mounted []string
	if googleSignInConfig(cfg).Enabled() {
		mounted = append(mounted, "google")
	}
	if microsoftSignInConfig(cfg).Enabled() {
		mounted = append(mounted, "microsoft")
	}
	return mounted
}

// trimTrailingSlash normalises a base before a path is appended, so the result
// never carries a doubled separator.
func trimTrailingSlash(base string) string {
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base
}
