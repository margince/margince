// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reading a company's own newsroom feed.
//
// Three fields per item and no fourth: the headline, the address, and the date.
// The article's TEXT is never stored — what a company published is theirs, and
// a signal that says "they announced a funding round, here is the link" is the
// whole product need. Caching the body would make this a copy of somebody's
// press page sitting in a CRM, which is the thing the enrichment position
// refuses on every other surface too.
//
// RSS 2.0 and Atom in one reader, because a newsroom is whichever its CMS
// emits and no reader of ours should care which.

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"
)

// FeedItem is one published item, reduced to what a signal carries.
type FeedItem struct {
	Title string
	URL   string
	// Published is the item's own date, or the zero time when the feed states
	// none. Zero is not "today": a signal dated by when we happened to read it
	// would put a three-year-old press release at the top of an account.
	Published time.Time
}

// feedMaxItems bounds one feed. A newsroom lists its recent items; a feed
// offering thousands is an archive, and reading it would file a decade of
// history as though it had just happened.
const feedMaxItems = 50

// feedMaxBytes bounds one fetch. Feeds are tens of kilobytes; the cap is what
// stops a hostile host from making a crawl budget disappear into one response.
const feedMaxBytes = 2 << 20 // 2 MiB

// rssDocument and atomDocument are the two shapes, read from one buffer. The
// field sets barely overlap, so one struct with everything optional would make
// every reader of it ask which half it got.
type rssDocument struct {
	XMLName xml.Name `xml:"rss"`
	Items   []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
		Date    string `xml:"date"` // dc:date, which some CMSes emit instead
	} `xml:"channel>item"`
}

type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

// ParseFeed reads whichever of the two shapes the bytes carry.
func ParseFeed(r io.Reader) ([]FeedItem, error) {
	body, err := io.ReadAll(io.LimitReader(r, feedMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading the feed: %w", err)
	}
	if len(body) > feedMaxBytes {
		return nil, fmt.Errorf("the feed exceeds %d bytes", feedMaxBytes)
	}
	if items, ok := parseRSS(body); ok {
		return items, nil
	}
	if items, ok := parseAtom(body); ok {
		return items, nil
	}
	return nil, fmt.Errorf("the response is neither an RSS nor an Atom feed")
}

func parseRSS(body []byte) ([]FeedItem, bool) {
	var doc rssDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, false
	}
	items := make([]FeedItem, 0, len(doc.Items))
	for _, item := range doc.Items {
		items = append(items, FeedItem{
			Title:     strings.TrimSpace(item.Title),
			URL:       strings.TrimSpace(item.Link),
			Published: parseFeedTime(item.PubDate, item.Date),
		})
	}
	return capFeedItems(items), true
}

func parseAtom(body []byte) ([]FeedItem, bool) {
	var doc atomDocument
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, false
	}
	items := make([]FeedItem, 0, len(doc.Entries))
	for _, entry := range doc.Entries {
		items = append(items, FeedItem{
			Title:     strings.TrimSpace(entry.Title),
			URL:       articleLink(entry.Links),
			Published: parseFeedTime(entry.Published, entry.Updated),
		})
	}
	return capFeedItems(items), true
}

// articleLink takes the alternate link, which is the article. An entry's other
// links point at the feed itself or at an edit endpoint, and neither is
// somewhere a reader wants to go.
func articleLink(links []atomLink) string {
	for _, link := range links {
		if link.Rel == "alternate" || link.Rel == "" {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

// capFeedItems drops what a feed states with no headline or no address — there
// is nothing to file and nowhere to send a reader — and bounds the rest.
func capFeedItems(items []FeedItem) []FeedItem {
	kept := items[:0]
	for _, item := range items {
		if item.Title == "" || item.URL == "" {
			continue
		}
		kept = append(kept, item)
		if len(kept) == feedMaxItems {
			break
		}
	}
	return kept
}

// feedTimeLayouts are the spellings a feed's date arrives in. RFC 1123 with and
// without a zone name is RSS; RFC 3339 is Atom; the rest are what real
// generators emit anyway.
var feedTimeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseFeedTime takes the first candidate that parses. An unparseable date is
// the ZERO time and not the current one: dating an item by when we read it
// would file a three-year-old press release as this week's news.
func parseFeedTime(candidates ...string) time.Time {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for _, layout := range feedTimeLayouts {
			if parsed, err := time.Parse(layout, candidate); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}
