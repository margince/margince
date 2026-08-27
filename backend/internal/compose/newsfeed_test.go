// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a company's newsroom actually serves, in both shapes.
//
// The load-bearing case is the one that looks like a detail: an item whose date
// will not parse gets the ZERO time rather than today's. Dating a press release
// by when we happened to read it would put three-year-old news at the top of an
// account, which is worse than having no date at all — a reader can see a blank
// and cannot see a wrong one.

import (
	"strings"
	"testing"
	"time"
)

func TestParseFeedReadsRSS(t *testing.T) {
	feed := `<?xml version="1.0"?>
	<rss version="2.0"><channel>
	  <title>Acme Newsroom</title>
	  <item>
	    <title>Acme raises a Series B</title>
	    <link>https://acme.example/news/series-b</link>
	    <pubDate>Tue, 12 Aug 2026 09:00:00 +0000</pubDate>
	  </item>
	  <item>
	    <title>Acme opens in Lisbon</title>
	    <link>https://acme.example/news/lisbon</link>
	    <pubDate>Mon, 04 Aug 2026 10:30:00 +0000</pubDate>
	  </item>
	</channel></rss>`

	items, err := ParseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Title != "Acme raises a Series B" {
		t.Errorf("title = %q", items[0].Title)
	}
	if items[0].URL != "https://acme.example/news/series-b" {
		t.Errorf("url = %q", items[0].URL)
	}
	if got := items[0].Published.UTC(); got != time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC) {
		t.Errorf("published = %v", got)
	}
}

func TestParseFeedReadsAtom(t *testing.T) {
	feed := `<?xml version="1.0"?>
	<feed xmlns="http://www.w3.org/2005/Atom">
	  <title>Globex Press</title>
	  <entry>
	    <title>Globex names a new CFO</title>
	    <link rel="alternate" href="https://globex.example/press/cfo"/>
	    <link rel="edit" href="https://globex.example/admin/press/cfo"/>
	    <published>2026-07-01T12:00:00Z</published>
	  </entry>
	</feed>`

	items, err := ParseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	// The ALTERNATE link is the article; the edit link is somewhere no reader
	// of ours should be sent.
	if items[0].URL != "https://globex.example/press/cfo" {
		t.Errorf("url = %q, want the alternate link", items[0].URL)
	}
}

func TestParseFeedLeavesAnUnreadableDateBlankRatherThanGuessing(t *testing.T) {
	feed := `<rss version="2.0"><channel><item>
	  <title>Undated announcement</title>
	  <link>https://acme.example/news/undated</link>
	  <pubDate>whenever</pubDate>
	</item></channel></rss>`

	items, err := ParseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if !items[0].Published.IsZero() {
		t.Errorf("published = %v, want the zero time — an item dated by when we read it reads as news", items[0].Published)
	}
}

func TestParseFeedDropsAnItemWithNothingToFile(t *testing.T) {
	feed := `<rss version="2.0"><channel>
	  <item><title>No link here</title></item>
	  <item><link>https://acme.example/news/no-title</link></item>
	  <item><title>Complete</title><link>https://acme.example/news/complete</link></item>
	</channel></rss>`

	items, err := ParseFeed(strings.NewReader(feed))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	// An item with no headline has nothing to say and one with no address has
	// nowhere to send a reader. Neither is a signal.
	if len(items) != 1 || items[0].Title != "Complete" {
		t.Fatalf("items = %+v, want only the complete one", items)
	}
}

func TestParseFeedCapsHowMuchOfAnArchiveItReads(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<rss version="2.0"><channel>`)
	for i := 0; i < feedMaxItems*3; i++ {
		b.WriteString(`<item><title>Item</title><link>https://acme.example/n</link></item>`)
	}
	b.WriteString(`</channel></rss>`)

	items, err := ParseFeed(strings.NewReader(b.String()))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	// A feed offering its whole archive is not a newsroom, and filing a decade
	// of history as though it had just happened is the failure being bounded.
	if len(items) != feedMaxItems {
		t.Errorf("items = %d, want the %d cap", len(items), feedMaxItems)
	}
}

func TestParseFeedRefusesWhatIsNotAFeed(t *testing.T) {
	for _, body := range []string{
		"<html><body>a newsroom PAGE, not its feed</body></html>",
		"not xml at all",
		"",
	} {
		if _, err := ParseFeed(strings.NewReader(body)); err == nil {
			t.Errorf("%q parsed as a feed, want a refusal", body)
		}
	}
}
