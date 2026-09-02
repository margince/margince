// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"net/url"
	"testing"
)

// The shape this whole feature exists for, byte for byte as anwr-group.com
// serves it: a document with no content but a refresh pointing at the real
// site. A reader that stops here sees an empty page and calls the domain
// parked.
const languageGatewayHTML = `<!DOCTYPE html>
<html lang="de">
  <head>
	<meta http-equiv="refresh" content="0; URL=/de">
    <meta charset="utf-8" />
  </head>
  <body></body>
</html>`

func TestARefreshOnlyLandingPageNamesWhereTheSiteReallyIs(t *testing.T) {
	base := mustParseURL(t, "https://www.anwr-group.com/")
	head := extractHeadAssets(languageGatewayHTML, base)
	if want := "https://www.anwr-group.com/de"; head.refresh != want {
		t.Fatalf("refresh = %q, want %q", head.refresh, want)
	}

	page := Page{Text: StripTags(languageGatewayHTML), Refresh: head.refresh}
	if !page.MetaRefreshOnly() {
		t.Fatalf("a page with no text and a refresh target must read as a trampoline, text = %q", page.Text)
	}
}

// A page with something to say is not a trampoline, even when it also carries
// a refresh: following it would abandon text the site did publish.
func TestAPageWithTextIsNotATrampolineEvenWhenItRefreshes(t *testing.T) {
	page := Page{Text: "ANWR Group is a retail cooperative.", Refresh: "https://acme.example/de"}
	if page.MetaRefreshOnly() {
		t.Fatal("a page carrying text must not be treated as a redirect shell")
	}
}

func TestRefreshTargetsOffTheSitesOwnDomainAreRefused(t *testing.T) {
	base := mustParseURL(t, "https://acme.example/")
	offSite := []struct {
		name string
		html string
	}{
		{
			name: "another company's server",
			html: `<meta http-equiv="refresh" content="0; URL=https://evil.test/pretend">`,
		},
		{
			// The classic suffix trick: acme.example.evil.test only LOOKS
			// like it belongs to acme.example.
			name: "a host that merely starts with the site's name",
			html: `<meta http-equiv="refresh" content="0; URL=https://acme.example.evil.test/">`,
		},
		{
			name: "a scheme that is not fetchable at all",
			html: `<meta http-equiv="refresh" content="0; URL=file:///etc/passwd">`,
		},
	}
	for _, tc := range offSite {
		t.Run(tc.name, func(t *testing.T) {
			if head := extractHeadAssets(tc.html, base); head.refresh != "" {
				t.Fatalf("refresh = %q, want nothing — the target leaves the site", head.refresh)
			}
		})
	}
}

// A refresh may point anywhere on the site's own domain, including another
// host under it — that is still the site we resolved to read.
func TestARefreshOntoTheSitesOwnDomainIsFollowed(t *testing.T) {
	base := mustParseURL(t, "https://acme.example/")
	head := extractHeadAssets(`<meta http-equiv="refresh" content="0; URL=https://www.acme.example/de">`, base)
	if want := "https://www.acme.example/de"; head.refresh != want {
		t.Fatalf("refresh = %q, want %q", head.refresh, want)
	}
}

func TestRefreshSpellingsTheWebActuallyUses(t *testing.T) {
	base := mustParseURL(t, "https://acme.example/")
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			name: "lowercase url and no space after the semicolon",
			html: `<meta http-equiv="refresh" content="0;url=/de">`,
			want: "https://acme.example/de",
		},
		{
			name: "an attribute name in capitals",
			html: `<meta HTTP-EQUIV="REFRESH" content="0; URL=/de">`,
			want: "https://acme.example/de",
		},
		{
			name: "a quoted target",
			html: `<meta http-equiv="refresh" content="0; url='/de'">`,
			want: "https://acme.example/de",
		},
		{
			// A delay is not a reason to ignore the target: the site is still
			// naming its real address, and the crawler is not waiting it out.
			name: "a delay before the redirect",
			html: `<meta http-equiv="refresh" content="10; URL=/de">`,
			want: "https://acme.example/de",
		},
		{
			// A reload names nowhere to go.
			name: "a refresh with no url at all",
			html: `<meta http-equiv="refresh" content="30">`,
			want: "",
		},
		{
			name: "a refresh naming an empty url",
			html: `<meta http-equiv="refresh" content="0; URL=">`,
			want: "",
		},
		{
			name: "some other http-equiv entirely",
			html: `<meta http-equiv="content-type" content="text/html; charset=utf-8">`,
			want: "",
		},
		{
			// The key must be exactly `url`. Accepting anything that merely
			// starts with it invents a redirect out of markup nobody wrote,
			// and the crawler pays a fetch for the misreading.
			name: "a key that only begins with url",
			html: `<meta http-equiv="refresh" content="garbage; url-not=/private">`,
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if head := extractHeadAssets(tc.html, base); head.refresh != tc.want {
				t.Fatalf("refresh = %q, want %q", head.refresh, tc.want)
			}
		})
	}
}

// HTML permits a page to omit </head>, and a page that does was still having
// its body markup read as the site's own declaration — which is how a refresh
// somebody else wrote could send the crawler somewhere.
func TestBodyMarkupIsIgnoredEvenWhenTheHeadIsNeverClosed(t *testing.T) {
	base := mustParseURL(t, "https://acme.example/")
	unclosed := []struct {
		name string
		html string
	}{
		{
			name: "a body opened without the head being closed",
			html: `<html><head><title>x</title><body>` +
				`<meta http-equiv="refresh" content="0; URL=/injected">` +
				`<meta property="og:image" content="/injected.png">` +
				`<link rel="icon" href="/injected-icon.png"></body></html>`,
		},
		{
			name: "a page with no head element at all",
			html: `<html><body><meta http-equiv="refresh" content="0; URL=/injected">` +
				`<link rel="icon" href="/injected-icon.png"></body></html>`,
		},
	}
	for _, tc := range unclosed {
		t.Run(tc.name, func(t *testing.T) {
			head := extractHeadAssets(tc.html, base)
			if head.refresh != "" {
				t.Errorf("refresh = %q, want nothing — it was declared in the body", head.refresh)
			}
			if head.ogImage != "" {
				t.Errorf("og:image = %q, want nothing — it was declared in the body", head.ogImage)
			}
			if len(head.icons) != 0 {
				t.Errorf("icons = %+v, want none — they were declared in the body", head.icons)
			}
		})
	}
}

// The first declaration wins, the same rule og:image follows: a page naming
// two destinations gives no basis to rank them, and a browser obeys the first.
func TestTheFirstRefreshDeclarationWins(t *testing.T) {
	base := mustParseURL(t, "https://acme.example/")
	head := extractHeadAssets(
		`<meta http-equiv="refresh" content="0; URL=/de">`+
			`<meta http-equiv="refresh" content="0; URL=/en">`, base)
	if want := "https://acme.example/de"; head.refresh != want {
		t.Fatalf("refresh = %q, want %q", head.refresh, want)
	}
}

// og:image and a refresh both arrive as <meta>, and the tokenizer yields a
// tag's attributes only once — so reading one must not consume the other.
func TestARefreshAndAnOpenGraphImageBothSurviveTheSameHead(t *testing.T) {
	base := mustParseURL(t, "https://acme.example/")
	head := extractHeadAssets(
		`<head><meta property="og:image" content="/share.png">`+
			`<meta http-equiv="refresh" content="0; URL=/de"></head>`, base)
	if want := "https://acme.example/share.png"; head.ogImage != want {
		t.Fatalf("og:image = %q, want %q", head.ogImage, want)
	}
	if want := "https://acme.example/de"; head.refresh != want {
		t.Fatalf("refresh = %q, want %q", head.refresh, want)
	}
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing %q: %v", rawURL, err)
	}
	return parsed
}
