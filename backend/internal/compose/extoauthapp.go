// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A unit's OAuth apps within the composed set. Here are the boot rules one
// declaration cannot check, the provider lookup, and the consent flow's URLs.

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/pkg/extension"
)

// extOAuthPathPrefix is where the consent flow of a unit's OAuth app is served,
// and the path its CSRF cookie is scoped to.
const extOAuthPathPrefix = "/v1/ext-oauth"

// coreOAuthNames are the provider names a unit's OAuth app may not take. The
// core vendors share the app store, and the capture providers share the signer.
func coreOAuthNames() []string {
	names := []string{providerIMAP}
	for _, p := range capture.AppProviders() {
		names = append(names, string(p))
	}
	return append(names, oauthProviders...)
}

// preflightOAuthApps runs the published Validate the generator also runs. It
// refuses a core name, and a provider some unit already declared.
func preflightOAuthApps(e extension.Extension, claimed map[string]extension.Name) error {
	for _, app := range e.OAuthApps {
		if err := app.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if slices.Contains(coreOAuthNames(), app.Provider) {
			return fmt.Errorf("compose: extension %q declares OAuth app %q, a name the core's own sign-in or capture uses — the two would share one stored app and one consent callback",
				e.Name, app.Provider)
		}
		if other, taken := claimed[app.Provider]; taken {
			return fmt.Errorf("compose: extension %q declares OAuth app %q, already declared by extension %q — one provider names one stored app",
				e.Name, app.Provider, other)
		}
		claimed[app.Provider] = e.Name
	}
	return nil
}

// composedOAuthApp answers the unit declaring an OAuth app for provider, and
// the app. It reads the set the boot validated, so a provider is unique.
func composedOAuthApp(provider string) (extension.Extension, extension.OAuthApp, bool) {
	for _, e := range ComposedExtensions() {
		for _, app := range e.OAuthApps {
			if app.Provider == provider {
				return e, app, true
			}
		}
	}
	return extension.Extension{}, extension.OAuthApp{}, false
}

// composedOAuthApps lists the OAuth apps the composed units declare, ordered
// by provider.
func composedOAuthApps() []extension.OAuthApp {
	var apps []extension.OAuthApp
	for _, e := range ComposedExtensions() {
		apps = append(apps, e.OAuthApps...)
	}
	slices.SortFunc(apps, func(a, b extension.OAuthApp) int { return strings.Compare(a.Provider, b.Provider) })
	return apps
}

// composedUnitApp is the lookup the app store serves unit apps through.
func composedUnitApp(provider string) (capture.UnitApp, bool) {
	e, app, ok := composedOAuthApp(provider)
	return capture.UnitApp{Unit: string(e.Name), Title: app.Title}, ok
}

// declaredOAuthApp answers one unit's own app for provider. Another unit's app
// is not found, so a unit cannot reach a neighbour's grant.
func declaredOAuthApp(unit, provider string) (extension.OAuthApp, bool) {
	e, app, ok := composedOAuthApp(provider)
	if !ok || string(e.Name) != unit {
		return extension.OAuthApp{}, false
	}
	return app, true
}

// extOAuthCallbackURL is a unit app's redirect_uri, on the bases a connector
// callback uses. Settings shows it before any flow runs, so one builder serves both.
func extOAuthCallbackURL(apiBaseURL, publicBaseURL, provider string) string {
	base := apiOrigin(apiBaseURL, publicBaseURL)
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + extOAuthPathPrefix + "/" + provider + "/callback"
}

// extOAuthLandingURL is the unit's own screen. The outcome and provider ride the
// hash query, which the router leaves off the route.
func extOAuthLandingURL(publicBaseURL string, unit extension.Name, provider, outcome string) string {
	query := url.Values{landingOutcomeParam: {outcome}, landingProviderParam: {provider}}
	return strings.TrimRight(publicBaseURL, "/") + "/#/ext/" + string(unit) + "?" + query.Encode()
}

// The hash query keys a unit screen reads the outcome from; the frontend's
// oauthReturnOf reads the same two.
const (
	landingOutcomeParam  = "oauth"
	landingProviderParam = "provider"
)

// extOAuthStateProvider is the provider a unit app's signed state and CSRF
// cookie carry. The hyphen is outside the provider grammar, so a state minted
// for a unit's app never verifies at a capture callback, or the reverse.
func extOAuthStateProvider(provider string) string {
	return "ext-" + provider
}
