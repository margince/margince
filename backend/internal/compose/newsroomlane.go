// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The newsroom lane of a deep read: find the company's own feed, read it, and
// file what it announced as signals on the account.
//
// It runs beside the logo lane and for the same reasons — a side fetch the
// crawl itself does not make, on the SAME guarded egress, under its own
// deadline. Nothing it does is allowed to hold the dossier open: a company that
// publishes no feed, a feed that will not parse, a host that stopped answering
// are all the ordinary case, and every one of them leaves the read exactly as
// it would have been.
//
// What it stores is in newsroomsignals.go and is deliberately small: a
// headline, a date, a link. The article's text never lands.

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
)

// newsroomLaneBudget is the whole lane's deadline — discovery, fetch and the
// row writes together. A newsroom is worth a few seconds and never worth the
// time the job budget reserves for CLOSING the dossier: a read cancelled before
// its outcome is recorded stays running forever, squatting the organization's
// one in-flight slot.
const newsroomLaneBudget = 20 * time.Second

// newsroomPaths are the addresses a feed sits at when a page did not declare
// one. Ordered by how often they answer, and short on purpose: each is a
// request against a host that may not want them, and a company with no feed
// should cost four 404s rather than forty.
var newsroomPaths = []string{"/feed", "/rss", "/news/feed", "/blog/feed", "/feed.xml"}

// readNewsroom files what the company announced about itself, if it says.
//
// Errors are logged and dropped rather than returned: this lane is additive to
// a read that has already succeeded, and a feed nobody could reach is not a
// failed enrichment.
func (w *siteDeepReadWorker) readNewsroom(ctx context.Context, claim people.SiteReadClaim, crawl siteCrawl) {
	if w.fetch == nil || claim.OrganizationID == nil {
		return
	}
	laneCtx, cancel := context.WithTimeout(ctx, newsroomLaneBudget)
	defer cancel()

	items := w.fetchNewsroomItems(laneCtx, crawl)
	if len(items) == 0 {
		return
	}
	orgID := *claim.OrganizationID
	events := make([]NewsroomItem, 0, len(items))
	for _, item := range items {
		events = append(events, NewsroomItem{
			Kind:      classifyHeadline(item.Title),
			Headline:  item.Title,
			URL:       item.URL,
			Published: item.Published,
		})
	}

	if err := database.WithWorkspaceTx(laneCtx, w.pool, func(tx pgx.Tx) error {
		raised, err := WriteNewsroomSignals(laneCtx, tx, orgID, events, time.Now())
		if err != nil {
			return err
		}
		if raised > 0 {
			w.log.InfoContext(laneCtx, "newsroom signals filed",
				"organization", orgID.String(), "raised", raised, "read", len(events))
		}
		return nil
	}); err != nil {
		w.log.WarnContext(laneCtx, "the newsroom lane could not file its signals",
			"organization", orgID.String(), "err", err)
	}
}

// fetchNewsroomItems tries the well-known addresses. The FIRST one that parses
// wins: a site offering several is
// offering the same newsroom twice, and reading both would file every
// announcement under two addresses.
func (w *siteDeepReadWorker) fetchNewsroomItems(ctx context.Context, crawl siteCrawl) []FeedItem {
	for _, candidate := range newsroomCandidates(crawl) {
		body, _, err := w.fetch.FetchAsset(ctx, candidate)
		if err != nil {
			// A 404 for a company with no feed is the ordinary case, and so is
			// a robots refusal. Neither is worth a warning on every crawl.
			w.log.DebugContext(ctx, "no newsroom feed here", "url", candidate, "err", err)
			continue
		}
		items, err := ParseFeed(strings.NewReader(string(body)))
		if err != nil {
			w.log.DebugContext(ctx, "the response was not a feed", "url", candidate, "err", err)
			continue
		}
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

// newsroomCandidates is where to look: the conventional paths off the site's
// own origin.
//
// A `<link rel="alternate" type="application/rss+xml">` in the seed page's head
// would be the better first answer, and it is not read here because the crawl
// hands this lane extracted TEXT rather than markup. Reaching for the head
// would mean a second fetch of a page already read, which is a cost this lane
// does not get to impose on a host — so it asks the four addresses a CMS
// actually uses and stops.
func newsroomCandidates(crawl siteCrawl) []string {
	origin := originOf(crawl.SeedURL)
	if origin == "" {
		return nil
	}
	candidates := make([]string, 0, len(newsroomPaths))
	for _, path := range newsroomPaths {
		candidates = append(candidates, origin+path)
	}
	return candidates
}

// originOf reduces an address to its scheme and host, which is what a
// well-known path hangs off.
func originOf(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
