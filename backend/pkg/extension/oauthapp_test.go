// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func aBooksApp() extension.OAuthApp {
	return extension.OAuthApp{
		Provider: "books",
		Title:    "Books ledger",
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: "https://auth.example.com/token",
		Scopes:   []string{"ledger.read"},
	}
}

// OnGrant is optional: a unit whose vendor returns nothing beyond the tokens
// has nothing to keep, and the core already stored the grant.
func TestAnOAuthAppWithoutOnGrantIsWellFormed(t *testing.T) {
	if err := aBooksApp().Validate(); err != nil {
		t.Fatalf("a well-formed app was refused: %v", err)
	}
}

func TestAnOAuthAppIsRefusedForEachMalformedField(t *testing.T) {
	for name, tc := range map[string]struct {
		edit func(*extension.OAuthApp)
		says string
	}{
		"kebab provider":        {func(a *extension.OAuthApp) { a.Provider = "books-ledger" }, "lower-case letters"},
		"empty provider":        {func(a *extension.OAuthApp) { a.Provider = "" }, "lower-case letters"},
		"provider too long":     {func(a *extension.OAuthApp) { a.Provider = strings.Repeat("b", 33) }, "longer than 32"},
		"blank title":           {func(a *extension.OAuthApp) { a.Title = "  " }, "no Title"},
		"http token endpoint":   {func(a *extension.OAuthApp) { a.TokenURL = "http://auth.example.com/token" }, "TokenURL"},
		"relative auth URL":     {func(a *extension.OAuthApp) { a.AuthURL = "/authorize" }, "AuthURL"},
		"auth URL with no host": {func(a *extension.OAuthApp) { a.AuthURL = "https:///authorize" }, "AuthURL"},
		"redirect_uri override": {func(a *extension.OAuthApp) {
			a.AuthParams = map[string]string{"redirect_uri": "https://partner.example/cb"}
		}, "redirect_uri"},
		"state override": {func(a *extension.OAuthApp) { a.AuthParams = map[string]string{"state": "x"} }, "state"},
	} {
		t.Run(name, func(t *testing.T) {
			app := aBooksApp()
			tc.edit(&app)
			err := app.Validate()
			if !errors.Is(err, extension.ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("the refusal %q does not name %q", err, tc.says)
			}
		})
	}
}

// A vendor's own consent parameter, such as one that asks for offline access,
// is what AuthParams is for.
func TestAnOAuthAppMayAddItsVendorsOwnAuthParams(t *testing.T) {
	app := aBooksApp()
	app.AuthParams = map[string]string{"access_type": "offline", "prompt": "consent"}
	if err := app.Validate(); err != nil {
		t.Fatalf("a vendor parameter was refused: %v", err)
	}
}
