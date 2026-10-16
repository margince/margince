// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// oauthUnitSource is a unit declaring the given OAuthApps slice elements.
func oauthUnitSource(entries string) string {
	return `package x

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

func keep(context.Context, extension.Runtime, extension.OAuthGrant) error { return nil }

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		OAuthApps: []extension.OAuthApp{
` + entries + `
		},
	}
}
`
}

const wholeOAuthApp = `			{
				Provider:   "books",
				Title:      "Books ledger",
				AuthURL:    "https://auth.ledgerly.test/authorize",
				TokenURL:   "https://auth.ledgerly.test/token",
				Scopes:     []string{"ledger.write", "ledger.read"},
				AuthParams: map[string]string{"prompt": "consent"},
				OnGrant:    keep,
			},
`

// An operator reads where an admin is sent and what the consent asks for. The
// consent detail and OnGrant stay in the source.
func TestOAuthAppsDeriveIntoManifest(t *testing.T) {
	derived, err := deriveSynthetic(t, "x", oauthUnitSource(wholeOAuthApp))
	if err != nil {
		t.Fatal(err)
	}
	s := string(derived)
	for _, want := range []string{
		`"oauth_apps"`,
		`"provider": "books"`,
		`"title": "Books ledger"`,
		`"auth_url": "https://auth.ledgerly.test/authorize"`,
		`"token_url": "https://auth.ledgerly.test/token"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("derived manifest misses %s:\n%s", want, s)
		}
	}
	if strings.Index(s, `"ledger.read"`) > strings.Index(s, `"ledger.write"`) {
		t.Errorf("scopes are not sorted:\n%s", s)
	}
	for _, absent := range []string{"keep", "prompt"} {
		if strings.Contains(s, absent) {
			t.Errorf("derived manifest carries %q:\n%s", absent, s)
		}
	}
}

// The generator refuses what boot refuses, so a unit does not pass generation
// and then fail to start.
func TestOAuthAppsRefuseWhatValidateRefuses(t *testing.T) {
	for name, tc := range map[string]struct{ entries, says string }{
		"an http token endpoint": {
			strings.Replace(wholeOAuthApp, "https://auth.ledgerly.test/token", "http://auth.ledgerly.test/token", 1), "TokenURL",
		},
		"a kebab provider": {strings.Replace(wholeOAuthApp, `"books"`, `"books-ledger"`, 1), "lower-case"},
		"no title":         {strings.Replace(wholeOAuthApp, `"Books ledger"`, `""`, 1), "Title"},
		"a provider twice": {wholeOAuthApp + wholeOAuthApp, "declared twice"},
		"a computed scope": {strings.Replace(wholeOAuthApp, `"ledger.read"`, `scope()`, 1), "Scopes entry"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := deriveSynthetic(t, "x", oauthUnitSource(tc.entries))
			if err == nil || !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("derive = %v, want a refusal naming %q", err, tc.says)
			}
		})
	}
}
