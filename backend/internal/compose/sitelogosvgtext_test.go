// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/webread"
)

func TestResolveLogoSkipsAnSVGWhoseWordsAreLiveTextForTheNextCandidate(t *testing.T) {
	site := &assetSite{assets: map[string][]byte{
		"https://acme.example/mark.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` +
			`<circle cx="50" cy="50" r="40"/><text x="10" y="50">Acme</text></svg>`),
		"https://acme.example/favicon.ico": logoFixture(t, 180, 180),
	}}
	logo, attempts := resolveCompanyLogo(context.Background(), site, logoSeed, declaredAssets{
		icons: []webread.IconRef{
			{URL: "https://acme.example/mark.svg", Rel: webread.RelIcon},
		},
	})
	if logo.SourceURL != "https://acme.example/favicon.ico" {
		t.Fatalf("chose %q, want the well-known favicon after the text SVG was skipped; attempts %+v", logo.SourceURL, attempts)
	}
}
