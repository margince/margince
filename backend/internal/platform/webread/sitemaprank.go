// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// What a sitemap's own metadata says about how much a page matters. The crawl
// adds the result to the rank of a URL its path classifier could not place.
// The shift stays below the lowest page-kind band there, so it reorders the
// unclassified tail and never outranks a page the path vocabulary recognizes.

import (
	"math"
	"time"
)

const (
	sitemapPriorityRange = 6 // <priority> 0..1 maps to -3..+3 around the protocol default 0.5
	sitemapFreshBonus    = 2 // modified within a year of the newest page the sitemap lists
	sitemapStalePenalty  = 2 // untouched for over two years before that
	sitemapFeedPenalty   = 2 // changes hourly or daily: a listing or feed, not an offer page
	sitemapFreshWindow   = 365 * 24 * time.Hour
	sitemapStaleWindow   = 2 * sitemapFreshWindow
)

// NewestLastMod is the reference "now" for judging freshness. Measured against
// the sitemap's own newest page instead of the wall clock, the same sitemap
// ranks the same way on every run.
func NewestLastMod(entries []SitemapEntry) time.Time {
	var newest time.Time
	for _, entry := range entries {
		if entry.LastMod.After(newest) {
			newest = entry.LastMod
		}
	}
	return newest
}

// RankNudge converts one entry's metadata into a rank shift against the
// sitemap's newest page. An entry with no metadata shifts by zero, so a site
// that publishes none ranks as before.
func (e SitemapEntry) RankNudge(newest time.Time) int {
	nudge := int(math.Round((e.Priority - defaultSitemapPriority) * sitemapPriorityRange))
	if !e.LastMod.IsZero() {
		switch age := newest.Sub(e.LastMod); {
		case age <= sitemapFreshWindow:
			nudge += sitemapFreshBonus
		case age > sitemapStaleWindow:
			nudge -= sitemapStalePenalty
		}
	}
	switch e.ChangeFreq {
	case "always", "hourly", "daily":
		nudge -= sitemapFeedPenalty
	}
	return nudge
}
