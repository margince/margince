// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Fingerprint is what one page DECLARES about the software serving it, in the
// places software conventionally announces itself: response headers, the names
// of the cookies it sets, the scripts it loads, and the generator its <head>
// names. It is raw observation with no vocabulary attached — matching these
// against a ruleset is the caller's job, so this package keeps owning HTTP
// mechanics and nothing else.
//
// The values are deliberately narrow. Cookie NAMES travel but their values do
// not: a session cookie's name identifies the shop system, its value is a
// visitor's session and belongs nowhere near a company record.
type Fingerprint struct {
	// URL is where the body came from after redirects, matching Page.FinalURL.
	URL string
	// Headers is the response header with Set-Cookie REMOVED, keys
	// canonicalized by net/http.
	//
	// The removal is the type's promise kept rather than a nicety: a
	// Set-Cookie value is a visitor's session, and callers match these headers
	// against rules and store the matched value as evidence on a company
	// record. CookieNames below carries the part that identifies software.
	Headers http.Header
	// CookieNames are the names of the cookies the response set, in the order
	// the response set them, deduplicated. Values are never carried.
	CookieNames []string
	// ScriptSrcs are the absolute src URLs of the <script> tags the page
	// loaded, deduplicated in document order.
	ScriptSrcs []string
	// Generator is the content of <meta name="generator">, or "" when the page
	// declared none.
	Generator string
	// Body is the raw HTML, for the rules that can only be seen in markup a
	// tag-level harvest does not reach (an inline bootstrap object, a themed
	// asset path). Capped by the same maxFetchBytes as every other fetch.
	Body string
}

// FetchFingerprint retrieves one page and reports what it declares about its
// own stack. It is the single-page read with the response's own metadata kept
// rather than discarded — the same SSRF guard, robots gate and byte cap as
// every other fetch, because it IS the same fetch.
//
// One page only: a fingerprint read is a single polite GET of a homepage, not
// a crawl, and this entry point cannot become one.
func (f *Fetcher) FetchFingerprint(ctx context.Context, rawURL string) (Fingerprint, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return Fingerprint{}, fmt.Errorf("webread: %q is not a fetchable URL", rawURL)
	}
	allowed, err := f.pathAllowed(ctx, parsed)
	if err != nil {
		return Fingerprint{}, err
	}
	if !allowed {
		return Fingerprint{}, fmt.Errorf("%w: %s", ErrRobotsDisallowed, parsed.Path)
	}
	// HTML only: the harvest below runs the HTML tokenizer, so a server must
	// not be allowed to negotiate markdown the tokenizer would read as text.
	got, err := f.getBytes(ctx, rawURL, acceptHTML, maxFetchBytes)
	if err != nil {
		return Fingerprint{}, err
	}
	if got.status != http.StatusOK {
		return Fingerprint{}, &StatusError{Status: got.status, URL: rawURL}
	}
	final := got.finalURL
	if final == nil {
		final = parsed
	}
	return fingerprintOf(got.header, string(got.body), final), nil
}

// fingerprintOf reads one fetched response for what it declares about its own
// stack. Split out of FetchFingerprint so the CRAWLER can build the same
// observation from the pages it already fetched: a shop system that only
// announces itself on /shop is invisible to a homepage-only read, and asking
// the site for those pages a second time to see headers the crawl already
// received would spend a request the site does not owe us.
func fingerprintOf(header http.Header, body string, final *url.URL) Fingerprint {
	scripts, generator := extractStackMarkers(body, final)
	return Fingerprint{
		URL:         final.String(),
		Headers:     withoutCookies(header),
		CookieNames: cookieNames(header),
		ScriptSrcs:  scripts,
		Generator:   generator,
		Body:        body,
	}
}

// withoutCookies copies the response header without Set-Cookie.
//
// A copy rather than a delete on the original, because the response header
// belongs to the http.Response the caller may still be reading.
func withoutCookies(header http.Header) http.Header {
	stripped := make(http.Header, len(header))
	for name, values := range header {
		if http.CanonicalHeaderKey(name) == "Set-Cookie" {
			continue
		}
		stripped[name] = values
	}
	return stripped
}

// cookieNames reads the Set-Cookie headers for their names alone.
//
// It parses through net/http rather than splitting on "=" so that a quoted or
// attribute-bearing cookie is read the way the standard reads it.
func cookieNames(header http.Header) []string {
	setCookies := (&http.Response{Header: header}).Cookies()
	seen := map[string]bool{}
	names := make([]string, 0, len(setCookies))
	for _, cookie := range setCookies {
		if cookie.Name == "" || seen[cookie.Name] {
			continue
		}
		seen[cookie.Name] = true
		names = append(names, cookie.Name)
	}
	return names
}

// extractStackMarkers harvests the two in-markup declarations a stack makes:
// the scripts the page loads and the generator it names. Both live in
// attributes StripTags destroys, so — like extractLinks — the harvest runs on
// the raw HTML.
//
// Unlike the icon harvest, this one does not stop at </head>: a shop system's
// telltale bundle is loaded at the end of <body> as often as in the head, and
// a script src is not an identity claim an untrusted body author can abuse the
// way a <link rel="icon"> would be — the worst a wrong one does is name a
// technology the page does load.
func extractStackMarkers(rawHTML string, base *url.URL) (scriptSrcs []string, generator string) {
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	seen := map[string]bool{}
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			// io.EOF or a malformed tail: the parseable prefix has been
			// harvested, which is all a best-effort discovery aid owes.
			return scriptSrcs, generator
		}
		name, hasAttr := tokenizer.TagName()
		if !hasAttr {
			continue
		}
		switch string(name) {
		case "script":
			if src, ok := absoluteAttr(tagAttrs(tokenizer)["src"], base); ok && !seen[src] {
				seen[src] = true
				scriptSrcs = append(scriptSrcs, src)
			}
		case "meta":
			attrs := tagAttrs(tokenizer)
			// First declaration wins, matching how the og:image harvest ranks
			// repeats: a page naming two generators offers no basis to choose.
			if generator == "" && strings.EqualFold(attrs["name"], "generator") {
				generator = strings.TrimSpace(attrs["content"])
			}
		}
	}
}

// absoluteAttr resolves one attribute against the page's own URL and keeps it
// only if it lands on http(s) — the same rule the link and icon harvests
// apply, so a javascript: or data: src is dropped rather than passed on as
// though it named a host.
func absoluteAttr(raw string, base *url.URL) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return "", false
	}
	resolved.Fragment = ""
	return resolved.String(), true
}
