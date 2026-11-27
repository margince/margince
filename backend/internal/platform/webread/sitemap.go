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
	"strconv"
	"strings"
	"time"
)

// SitemapEntry is one <url> (or child <sitemap>) of a sitemap.xml: the address
// and what the site says about it. LastMod is the zero time when the site gave
// none or wrote one we cannot read. Priority is 0.5, the protocol's own
// default, when absent or out of range.
type SitemapEntry struct {
	Loc        string
	LastMod    time.Time
	ChangeFreq string
	Priority   float64
}

const defaultSitemapPriority = 0.5

// FetchSitemap retrieves <origin>/sitemap.xml (robots-checked like any path)
// and returns its entries. Both shapes parse. A urlset yields page URLs. A
// sitemapindex yields the child sitemap URLs as-is and is not recursed.
// The crawl's discovery budget does not chase nested indexes. The caller
// ignores entries that are sitemaps rather than pages. A missing
// sitemap (4xx) is an empty list with no error, because most sites have none.
func (f *Fetcher) FetchSitemap(ctx context.Context, origin string) ([]SitemapEntry, error) {
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
		return parseSitemapEntries(body)
	case status >= 400 && status < 500:
		return nil, nil // no sitemap declared — absence is normal
	default:
		return nil, fmt.Errorf("webread: sitemap.xml answered %d", status)
	}
}

// parseSitemapEntries collects every <url> or <sitemap> element's <loc> with
// the metadata beside it. It walks the token stream instead of unmarshalling
// a struct, so one pass reads both shapes. The element wrapping a <loc>
// differs, the <loc> does not.
func parseSitemapEntries(body string) ([]SitemapEntry, error) {
	decoder := xml.NewDecoder(strings.NewReader(body))
	var entries []SitemapEntry
	current := SitemapEntry{Priority: defaultSitemapPriority}
	var field, text strings.Builder
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("webread: sitemap.xml is not XML: %w", err)
		}
		switch element := token.(type) {
		case xml.StartElement:
			field.Reset()
			text.Reset()
			field.WriteString(element.Name.Local)
		case xml.CharData:
			text.Write(element)
		case xml.EndElement:
			switch element.Name.Local {
			case "url", "sitemap":
				if current.Loc != "" {
					entries = append(entries, current)
				}
				current = SitemapEntry{Priority: defaultSitemapPriority}
			default:
				if field.String() == element.Name.Local {
					current.setField(element.Name.Local, strings.TrimSpace(text.String()))
				}
			}
			field.Reset()
			text.Reset()
		}
	}
}

// setField stores one element's text. Unreadable metadata is dropped rather
// than failing the sitemap: the address is the part the crawl needs.
func (e *SitemapEntry) setField(name, value string) {
	switch name {
	case "loc":
		e.Loc = value
	case "lastmod":
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, value); err == nil {
				e.LastMod = parsed
				return
			}
		}
	case "changefreq":
		e.ChangeFreq = strings.ToLower(value)
	case "priority":
		if parsed, err := strconv.ParseFloat(value, 64); err == nil && parsed >= 0 && parsed <= 1 {
			e.Priority = parsed
		}
	}
}
