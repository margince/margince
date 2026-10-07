// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What one deep read may fetch, and the only direction a limit may move it.

import (
	"time"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// The defaults are the settings' own, so a read with no opinion and an
// installation nobody has tuned crawl alike.
const (
	defaultCrawlMaxPages = capture.DefaultSiteReadMaxPages
	defaultCrawlMaxBytes = capture.DefaultSiteReadMaxMiB << 20
	defaultCrawlWall     = capture.DefaultSiteReadWallSeconds * time.Second
)

// CrawlCaps bounds one deep read. The zero value means "the defaults", so a
// caller with no opinion — the siteread debug loop, a test — crawls as an
// untuned installation does.
type CrawlCaps struct {
	MaxPages int
	MaxBytes int
	Wall     time.Duration
}

// crawlCeiling is the most an admin may let one read fetch. The worker's
// crawler is built at the ceiling and narrowed to the chosen limits per read,
// because narrowing is the only direction within takes.
var crawlCeiling = CrawlCaps{
	MaxPages: capture.MaxSiteReadMaxPages,
	MaxBytes: capture.MaxSiteReadMaxMiB << 20,
	Wall:     capture.MaxSiteReadWallSeconds * time.Second,
}

// crawlCapsFrom converts the admin's limits into the crawler's units.
func crawlCapsFrom(limits capture.SiteReadLimits) CrawlCaps {
	return CrawlCaps{
		MaxPages: limits.MaxPages,
		MaxBytes: limits.MaxMiB << 20,
		Wall:     time.Duration(limits.WallSeconds) * time.Second,
	}
}

func (c CrawlCaps) withDefaults() CrawlCaps {
	if c.MaxPages <= 0 {
		c.MaxPages = defaultCrawlMaxPages
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = defaultCrawlMaxBytes
	}
	if c.Wall <= 0 {
		c.Wall = defaultCrawlWall
	}
	return c
}

// withPageCeiling returns a crawler that reads at most pages, or the receiver
// unchanged when the ceiling is not lower. Narrowing only: a per-run cap is a
// request to read LESS, and honouring one that asked for more would let a job
// payload raise a deployment's own limit.
func (c *siteCrawler) withPageCeiling(pages int) *siteCrawler {
	if pages <= 0 || pages >= c.maxPages {
		return c
	}
	narrowed := *c
	narrowed.maxPages = pages
	return &narrowed
}

// within returns a crawler bounded by caps wherever caps is the tighter, so a
// limit can only ever narrow what the crawler was built with. A zero field
// leaves that bound alone.
func (c *siteCrawler) within(caps CrawlCaps) *siteCrawler {
	narrowed := *c
	if caps.MaxPages > 0 && caps.MaxPages < narrowed.maxPages {
		narrowed.maxPages = caps.MaxPages
	}
	if caps.MaxBytes > 0 && caps.MaxBytes < narrowed.maxBytes {
		narrowed.maxBytes = caps.MaxBytes
	}
	if caps.Wall > 0 && caps.Wall < narrowed.wall {
		narrowed.wall = caps.Wall
	}
	return &narrowed
}
