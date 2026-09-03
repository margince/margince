// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// sitemap.xml: the one discovery channel a site publishes for crawlers rather
// than for readers. Its own file because it is the only fetch here that parses
// XML rather than HTML, and the only one whose result is a list of addresses
// rather than a page.

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

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
