// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"net/url"
	"testing"
)

func TestTheHeadNamesTheSiteByOpenGraphThenTitle(t *testing.T) {
	base, err := url.Parse("https://acme.example/")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`<head><title>Home | Acme</title><meta property="og:site_name" content=" Acme  Robotics "></head>`: "Acme Robotics",
		`<head><title>Lucky   Spin Casino</title><meta property="og:title" content="Welcome"></head>`:      "Lucky Spin Casino",
		`<head><meta name="description" content="x"></head><body><title>Body</title></body>`:               "",
	}
	for html, want := range cases {
		if got := extractHeadAssets(html, base).naming.siteNameOrTitle(); got != want {
			t.Errorf("siteNameOrTitle(%s) = %q, want %q", html, got, want)
		}
	}
}
