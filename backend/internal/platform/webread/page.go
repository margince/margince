// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/publicsuffix"
)

// Page is one fetched page with the crawler-facing extras Fetch discards: the
// nav links the raw HTML carried, the visual-identity assets its <head>
// declared, and the raw byte count (for the crawl's total byte budget — the
// stripped text under-counts what was transferred).
type Page struct {
	URL string
	// FinalURL is where the body actually came from: the requested URL unless
	// the server redirected, in which case it names the last hop. A crawler
	// deciding what counts as on-site must judge by this one — the requested
	// host may only forward.
	FinalURL string
	Text     string
	Links    []string
	Bytes    int
	// OGImage is the og:image the page declared (absolute), or "" when it
	// declared none.
	OGImage string
	// Icons are the icons the page's <link rel> declared, in document order.
	Icons []IconRef
	// Fingerprint is what this page declared about the software serving it,
	// read from the same response the text above came from.
	//
	// Carried on every crawled page rather than only the homepage: a shop
	// system, a portal or a careers platform frequently announces itself on
	// the one page that runs it and nowhere else, and the crawl has already
	// paid for that page.
	Fingerprint Fingerprint
	// Refresh is the <meta http-equiv="refresh"> target this page declared
	// (absolute), or "" when it declared none, none that resolves, or one
	// leaving the page's own registrable domain. A caller follows it only
	// after finding nothing else to read — see MetaRefreshOnly.
	Refresh string
}

// MetaRefreshOnly reports whether this page is a redirect trampoline: it
// declares a refresh target and carries no readable text of its own.
//
// This is a real shape, not a defensive one. A site can announce its language
// choice this way — an empty document whose whole content is
// `<meta http-equiv="refresh" content="0; URL=/de">` — and a browser lands on
// the real site without the person ever seeing the shell. A reader that stops
// at the shell sees a page with nothing on it, which is indistinguishable from
// a parked domain and gets judged as one.
func (p Page) MetaRefreshOnly() bool {
	return p.Refresh != "" && strings.TrimSpace(p.Text) == ""
}

// Icon rel kinds a page can declare. Callers rank by kind rather than by
// re-parsing rel strings.
const (
	// RelIcon is the standard favicon link (rel="icon" or "shortcut icon").
	RelIcon = "icon"
	// RelAppleTouchIcon is the homescreen icon: usually the largest square
	// raster mark a site publishes.
	RelAppleTouchIcon = "apple-touch-icon"
)

// IconRef is one icon a page's <head> declared: the absolute URL, the rel
// kind that declared it, and the sizes attribute lowercased ("180x180",
// "any", or "" when the page did not say).
type IconRef struct {
	URL   string
	Rel   string
	Sizes string
}

// FetchPage retrieves one page for the crawler: stripped text plus the <a href>
// targets it carried. It requests HTML (never markdown) so link harvesting works
// — the single-page Fetch may return verbatim markdown when a server offers it,
// but the crawler's stripped text is unchanged. The harvest runs on the RAW HTML
// before StripTags (stripping destroys hrefs). Links come back absolute (resolved
// against the page URL), http(s)-only, fragment-free, and deduplicated in
// document order.
func (f *Fetcher) FetchPage(ctx context.Context, rawURL string) (Page, error) {
	if parsed, err := url.Parse(rawURL); err != nil || parsed.Host == "" {
		return Page{}, fmt.Errorf("webread: %q is not a fetchable URL", rawURL)
	}
	// HTML only — the crawler needs HTML for the <a href> link harvest.
	// `base` is where the body CAME from, not where it was asked for: a bare
	// domain redirecting to its www host is the ordinary case, and resolving
	// the page's own relative references against the pre-redirect origin would
	// point every one of them at a host that never served this page.
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return Page{}, fmt.Errorf("webread: %q is not a fetchable URL", rawURL)
	}
	allowed, err := f.pathAllowed(ctx, parsed)
	if err != nil {
		return Page{}, err
	}
	if !allowed {
		return Page{}, fmt.Errorf("%w: %s", ErrRobotsDisallowed, parsed.Path)
	}
	// getBytes rather than fetchDoc, because the RESPONSE HEADER is half a
	// fingerprint and fetchDoc keeps only the media type. Same guard, same
	// robots gate, same cap — one fetch that keeps what it already received.
	got, err := f.getBytes(ctx, rawURL, acceptHTML, maxFetchBytes)
	if err != nil {
		return Page{}, err
	}
	if got.status != http.StatusOK {
		return Page{}, &StatusError{Status: got.status, URL: rawURL}
	}
	base := got.finalURL
	if base == nil {
		base = parsed
	}
	body := string(got.body)
	head := extractHeadAssets(body, base)
	return Page{
		URL:         rawURL,
		FinalURL:    base.String(),
		Text:        StripTags(body),
		Links:       extractLinks(body, base),
		Bytes:       len(body),
		OGImage:     head.ogImage,
		Icons:       head.icons,
		Refresh:     head.refresh,
		Fingerprint: fingerprintOf(got.header, body, base),
	}, nil
}

// headAssets is what a page's <head> declared about itself: its visual
// identity, and where it says the reader should go instead.
type headAssets struct {
	ogImage string
	icons   []IconRef
	refresh string
}

// extractHeadAssets harvests what a page's <head> declares about itself: its
// og:image, every <link rel> naming an icon, and any meta-refresh target. All
// live in attributes StripTags destroys, so — like extractLinks — the harvest
// runs on the raw HTML. URLs come back absolute; a candidate whose href does
// not resolve to an http(s) URL is dropped rather than guessed at.
func extractHeadAssets(rawHTML string, base *url.URL) headAssets {
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	var head headAssets
	seen := map[string]bool{}
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			// io.EOF or a malformed tail: the parseable prefix has been
			// harvested, which is all a best-effort discovery aid owes.
			return head
		}
		name, hasAttr := tokenizer.TagName()
		// The harvest stops where the head does, because that is where a site
		// declares its own identity. Markup in the body is not the same claim:
		// on a page that renders user-generated content, whoever wrote that
		// content also wrote the tags around it, and a <link rel="icon"> down
		// there would let them choose the company's face — or, for a refresh,
		// where the crawler goes next.
		//
		// A page may legally omit </head>, so <body> ends it too. Testing only
		// for the closing tag let body markup on such a page be read as the
		// site's own declaration, which is the whole thing this boundary
		// exists to prevent.
		if tokenType == html.EndTagToken && string(name) == "head" {
			return head
		}
		if tokenType == html.StartTagToken && string(name) == "body" {
			return head
		}
		if !hasAttr {
			continue
		}
		switch string(name) {
		case "meta":
			// A <meta> is at most one of these, but which one is only known
			// after its attributes are read, and the tokenizer yields those
			// once — so both readings take the attribute set, not the
			// tokenizer.
			attrs := tagAttrs(tokenizer)
			if found, ok := ogImageFrom(attrs, base); ok && head.ogImage == "" {
				// First declaration wins: a page repeating og:image offers no
				// basis to rank the repeats, and the first is conventionally
				// the canonical one.
				head.ogImage = found
			}
			if found, ok := refreshFrom(attrs, base); ok && head.refresh == "" {
				head.refresh = found
			}
		case "link":
			if icon, ok := iconFrom(tokenizer, base); ok && !seen[icon.URL] {
				seen[icon.URL] = true
				head.icons = append(head.icons, icon)
			}
		}
	}
}

// tagAttrs collects the current tag's attributes, keys lowercased. The
// tokenizer yields them one at a time and only for the tag it is positioned
// on, so callers that need two attributes together (rel AND href) read the
// whole set first.
func tagAttrs(tokenizer *html.Tokenizer) map[string]string {
	attrs := map[string]string{}
	for {
		key, value, more := tokenizer.TagAttr()
		attrs[strings.ToLower(string(key))] = string(value)
		if !more {
			return attrs
		}
	}
}

// ogImageFrom reads one <meta>'s attributes as an og:image declaration. Open
// Graph names the field in `property`; some CMSs emit it in `name`, so both
// are accepted.
func ogImageFrom(attrs map[string]string, base *url.URL) (string, bool) {
	if attrs["property"] != "og:image" && attrs["name"] != "og:image" {
		return "", false
	}
	return resolveAsset(base, attrs["content"])
}

// refreshFrom reads one <meta>'s attributes as a refresh declaration, and
// returns the absolute URL it points at.
//
// The target must stay on the page's own registrable domain. A refresh is
// markup, so whoever writes the page chooses it, and following one off-site
// would let any page we read redirect the crawl at a host we never decided to
// visit — including one chosen to be read as the company's own site. Staying
// on-domain keeps the follow within the site we already resolved to read, so
// it can reach `/de` on the same host but never someone else's server.
//
// Only a same-document `URL=` target is refused too: a refresh that names no
// URL is a page telling itself to reload, which is not somewhere else to go.
func refreshFrom(attrs map[string]string, base *url.URL) (string, bool) {
	if !strings.EqualFold(strings.TrimSpace(attrs["http-equiv"]), "refresh") {
		return "", false
	}
	target, ok := refreshTarget(attrs["content"])
	if !ok {
		return "", false
	}
	resolved, ok := resolveAsset(base, target)
	if !ok {
		return "", false
	}
	if !SameRegistrableDomain(base.String(), resolved) {
		return "", false
	}
	return resolved, true
}

// refreshTarget pulls the URL out of a refresh content attribute. The value is
// a delay, optionally followed by `; url=<target>` — `0; URL=/de` is the
// ordinary spelling. The delay is not read: a site that says to leave after
// ten seconds is still naming somewhere else as its real address, and the
// crawler is not a browser waiting out a countdown.
func refreshTarget(content string) (string, bool) {
	_, after, found := strings.Cut(content, ";")
	if !found {
		return "", false
	}
	key, target, found := strings.Cut(after, "=")
	if !found {
		return "", false
	}
	// The key must be exactly `url`, not merely start with it. Accepting any
	// `url…=` spelling turned `garbage; url-not=/private` into a redirect
	// nobody wrote, and every one of those is a fetch the crawler makes on
	// markup it misread.
	if !strings.EqualFold(strings.TrimSpace(key), "url") {
		return "", false
	}
	// Quotes are common in hand-written markup and are not part of the URL.
	return strings.Trim(strings.TrimSpace(target), `"'`), true
}

// iconFrom reads one <link> as an icon declaration.
func iconFrom(tokenizer *html.Tokenizer, base *url.URL) (IconRef, bool) {
	attrs := tagAttrs(tokenizer)
	rel := iconRel(attrs["rel"])
	if rel == "" {
		return IconRef{}, false
	}
	href, ok := resolveAsset(base, attrs["href"])
	if !ok {
		return IconRef{}, false
	}
	return IconRef{URL: href, Rel: rel, Sizes: strings.ToLower(strings.TrimSpace(attrs["sizes"]))}, true
}

// iconRel reduces a rel attribute to the icon kind it declares, or "" for a
// link that declares none. rel is a space-separated token list, so
// "shortcut icon" is an icon. rel="mask-icon" is deliberately not a kind: it
// is a monochrome stencil for Safari's pinned tabs, never the company's mark.
func iconRel(rel string) string {
	for _, token := range strings.Fields(strings.ToLower(rel)) {
		switch token {
		case RelAppleTouchIcon, "apple-touch-icon-precomposed":
			return RelAppleTouchIcon
		case RelIcon:
			return RelIcon
		}
	}
	return ""
}

// resolveAsset resolves a declared asset reference. Unlike a nav link, an
// empty reference is not the page itself — a <link href=""> declares no asset,
// so it is dropped instead of resolving to the page's own URL.
func resolveAsset(base *url.URL, ref string) (string, bool) {
	if strings.TrimSpace(ref) == "" {
		return "", false
	}
	return resolveLink(base, ref)
}

// extractLinks harvests <a href> targets from raw HTML. The tokenizer treats
// <script>/<style> contents as raw text, so an href spelled inside a script
// string is never harvested — only real anchor elements count.
func extractLinks(rawHTML string, base *url.URL) []string {
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	seen := map[string]bool{}
	var links []string
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			// io.EOF or a malformed tail: either way the parseable prefix has
			// been harvested, which is all a best-effort discovery aid owes.
			return links
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := tokenizer.TagName()
		if string(name) != "a" || !hasAttr {
			continue
		}
		for {
			key, value, more := tokenizer.TagAttr()
			if string(key) == "href" {
				if link, ok := resolveLink(base, string(value)); ok && !seen[link] {
					seen[link] = true
					links = append(links, link)
				}
			}
			if !more {
				break
			}
		}
	}
}

// resolveLink turns one href into an absolute, fragment-free http(s) URL, or
// reports it unusable (mailto:, javascript:, malformed, hostless).
func resolveLink(base *url.URL, href string) (string, bool) {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", false
	}
	abs := base.ResolveReference(ref)
	if (abs.Scheme != "http" && abs.Scheme != "https") || abs.Host == "" {
		return "", false
	}
	abs.Fragment = ""
	return abs.String(), true
}

// FetchSitemap retrieves <origin>/sitemap.xml (robots-checked like any path)
// and returns its <loc> entries. Both shapes parse: a urlset yields page URLs;
// a sitemapindex yields the CHILD SITEMAP URLs as-is — deliberately not
// recursed, the crawl's discovery budget does not chase nested indexes, and
// the caller is expected to ignore entries that are sitemaps rather than
// pages. A missing sitemap (4xx) is an empty list with no error: most sites
// have none, absence is normal.
func (f *Fetcher) FetchSitemap(ctx context.Context, origin string) ([]string, error) {
	sitemapURL := strings.TrimSuffix(origin, "/") + "/sitemap.xml"
	parsed, err := url.Parse(sitemapURL)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("webread: %q is not a fetchable origin", origin)
	}
	allowed, err := f.pathAllowed(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, fmt.Errorf("%w: %s", ErrRobotsDisallowed, parsed.Path)
	}

	body, status, _, err := f.getRaw(ctx, sitemapURL, "")
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusOK:
		return parseSitemapLocs(body)
	case status >= 400 && status < 500:
		return nil, nil // no sitemap declared — absence is normal
	default:
		return nil, fmt.Errorf("webread: sitemap.xml answered %d", status)
	}
}

// parseSitemapLocs collects every <loc>'s text. Walking the token stream
// instead of unmarshalling a struct lets one pass read both the urlset and
// sitemapindex shapes — the element carrying a <loc> differs, the <loc> does
// not.
func parseSitemapLocs(body string) ([]string, error) {
	decoder := xml.NewDecoder(strings.NewReader(body))
	var locs []string
	inLoc := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return locs, nil
		}
		if err != nil {
			return nil, fmt.Errorf("webread: sitemap.xml is not XML: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			inLoc = element.Name.Local == "loc"
		case xml.EndElement:
			inLoc = false
		case xml.CharData:
			if inLoc {
				if loc := strings.TrimSpace(string(element)); loc != "" {
					locs = append(locs, loc)
				}
			}
		}
	}
}

// SameRegistrableDomain reports whether two URLs' hostnames share an eTLD+1
// (publicsuffix), the "same site" test the crawler's off-domain gate uses:
// blog.acme.de and www.acme.de are both acme.de; acme.de and acme.com are
// not, and neither are two customers of the same co.uk-style suffix. Any
// parse failure answers false — an unparseable URL is never "same site".
func SameRegistrableDomain(a, b string) bool {
	domainA, okA := registrableDomain(a)
	domainB, okB := registrableDomain(b)
	return okA && okB && domainA == domainB
}

func registrableDomain(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return "", false
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(parsed.Hostname()))
	if err != nil {
		return "", false
	}
	return domain, true
}
