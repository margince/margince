// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reaching a company's site when the one URL we derived from its domain does
// not answer.
//
// A domain becomes a seed as `https://<domain>` and nothing else (people's
// EnrichTargetURL, capture's auto-enrich). That is the right first guess and
// the wrong only guess: a site can serve TLS on www but not on the apex, or
// have no TLS at all. On a real import of 162 companies, 37 site reads died on
// the seed fetch having read zero pages, and half of those answer perfectly
// well on another host or scheme — so half the companies with no logo, no
// facts and no profile had a reachable website the whole time.
//
// This is the ladder a browser walks when a person types a bare domain, and
// nothing more: the same site, named the way that site actually publishes it.

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/platform/webread"
)

// fetchSeed gets the landing page, or reports why the site could not be read
// at all. It returns the URL that ANSWERED, which is the site's own spelling
// of itself and what the crawl treats as on-site from then on.
func (c *siteCrawler) fetchSeed(ctx context.Context, pacer crawlPacer, seedURL string) (seedRead, error) {
	page, err := c.fetchPaced(ctx, pacer, seedURL)
	if transientCrawlError(ctx, err) {
		// The landing page is the only irreplaceable discovery source. One
		// immediate retry absorbs a transient edge/CDN timeout while the
		// crawl's wall deadline still bounds the attempt.
		page, err = c.fetchPaced(ctx, pacer, seedURL)
	}
	if err == nil {
		return c.settleSeed(ctx, pacer, seedURL, page), nil
	}
	if errors.Is(err, webread.ErrRobotsDisallowed) {
		// A refused seed carries no body, so there is nothing to follow and
		// nothing to read: the refusal itself is the answer.
		answered := answeredSeedURL(seedURL, page)
		c.applyCrawlDelay(pacer, answered)
		return seedRead{URL: answered, Page: page}, err
	}
	for _, candidate := range seedFallbacks(seedURL) {
		if ctx.Err() != nil {
			break
		}
		retryPage, retryErr := c.fetchPaced(ctx, pacer, candidate)
		if transientCrawlError(ctx, retryErr) {
			// The same single retry the first spelling gets. A CDN timeout on
			// the www spelling is not evidence that the site is unreachable,
			// and treating it as one loses a company its whole read.
			retryPage, retryErr = c.fetchPaced(ctx, pacer, candidate)
		}
		if retryErr == nil {
			return c.settleSeed(ctx, pacer, candidate, retryPage), nil
		}
		// A refusal is the site's answer, not a spelling that failed to
		// resolve. Every remaining candidate is the SAME site under another
		// host or scheme, so trying them would be answering a "no" by
		// knocking on the next door.
		if errors.Is(retryErr, webread.ErrRobotsDisallowed) {
			return seedRead{URL: seedURL}, retryErr
		}
	}
	return seedRead{URL: seedURL}, err
}

// seedRead is the landing page a seed resolved to: the URL that answered, the
// body, and whether the site named an address the crawl could not reach.
type seedRead struct {
	URL  string
	Page webread.Page
	// UnresolvedForward is set when the landing page named somewhere else as
	// the site's real address and that address could not be read. The page
	// below is then the forwarding shell — empty, but empty for a reason that
	// is not "this domain is parked".
	UnresolvedForward bool
}

// settleSeed turns a seed that answered into the page the crawl should treat
// as the landing page, following a meta-refresh trampoline once if that is
// what answered.
//
// A site can publish its real address in markup instead of an HTTP redirect —
// an empty document whose whole content is
// `<meta http-equiv="refresh" content="0; URL=/de">`. The fetcher follows HTTP
// redirects and never sees this one, so the crawl reads a page with no text on
// it. That is indistinguishable from a parked domain, and the domain triage
// judges it as one: anwr-group.com was refused a company on exactly this
// shape, and every language-gateway site of that vintage has it.
//
// Followed ONCE, never in a loop. One hop reaches the site a browser would
// land on, which is the whole gap; chaining them would let a site walk the
// crawler through as many fetches as it cares to write, and a cycle of two
// pages pointing at each other would never end. A refresh that leaves the
// site's own registrable domain was already dropped at parse time
// (webread.refreshFrom), so the hop stays on the site we resolved to read.
// A follow that does not arrive says so, in UnresolvedForward. The shell is
// then still empty, but the triage must not read that emptiness as a parked
// domain: the site named an address, and failing to reach it is a gap in the
// read rather than an answer about the company.
func (c *siteCrawler) settleSeed(ctx context.Context, pacer crawlPacer, requested string, page webread.Page) seedRead {
	answered := answeredSeedURL(requested, page)
	c.applyCrawlDelay(pacer, answered)
	if !page.MetaRefreshOnly() {
		return seedRead{URL: answered, Page: page}
	}
	unresolved := seedRead{URL: answered, Page: page, UnresolvedForward: true}
	landed, err := c.fetchPaced(ctx, pacer, page.Refresh)
	if err != nil {
		// The trampoline is all this site gave us.
		return unresolved
	}
	followed := answeredSeedURL(page.Refresh, landed)
	// Where the hop LANDED, not just where it pointed. The parse-time check
	// judged the target URL, and a target on the site's own domain can still
	// 30x onto somebody else's — an open redirect is an ordinary thing to find
	// on a corporate site. Letting that through would hand the crawl a
	// boundary, and a company identity, chosen by whoever wrote the markup.
	//
	// A seed that redirects off-domain by HTTP is different and stays allowed
	// (answeredSeedURL): there the destination is where the domain WE were
	// asked to read forwards to, which is that domain's own answer about
	// where it lives.
	if !webread.SameRegistrableDomain(answered, followed) {
		return unresolved
	}
	c.applyCrawlDelay(pacer, followed)
	// A second forwarding shell is not followed again — one hop is the cap —
	// so it leaves the read unresolved for the same reason a failed fetch does.
	return seedRead{URL: followed, Page: landed, UnresolvedForward: landed.MetaRefreshOnly()}
}

// answeredSeedURL is the URL the seed's body actually came from. The fetch
// follows redirects the ladder never sees — a domain answering 301 onto
// another host "answers" in the ladder's eyes while serving nothing itself —
// so the crawl boundary, the probe origin and the favicon derivation must all
// name the destination, not the forwarder. A page without a final URL — the
// robots-refused seed carries none — answers where it was asked.
func answeredSeedURL(requested string, page webread.Page) string {
	if page.FinalURL != "" {
		return page.FinalURL
	}
	return requested
}

// seedFallbacks returns the other spellings of a seed worth trying, in order,
// after the seed itself has failed to answer. The seed is never repeated.
//
// Only the host and the scheme vary. A different PATH would be a different
// page and a different REGISTRABLE DOMAIN a different company, so neither is a
// fallback — it would be a guess about which site we meant.
//
// Downgrading to http is last and deliberate. Plain http is worth trying
// because a small site's marketing page is public either way and the crawl
// reads nothing private; it goes last so a working https is always preferred.
func seedFallbacks(seedURL string) []string {
	parsed, err := url.Parse(seedURL)
	if err != nil || parsed.Host == "" {
		return nil
	}
	if parsed.Scheme != schemeHTTP && parsed.Scheme != schemeHTTPS {
		return nil
	}
	// Only the `www` convention, and only ever by ADDING it or removing that
	// exact label. Stripping any other label would point at a different host
	// that may serve someone else — `app.acme.com` is not `acme.com`.
	//
	// Adding www is offered for every host, including multi-label ones:
	// counting dots called `acme.co.uk` a subdomain and skipped the www
	// spelling, which is where a good share of UK and German companies
	// actually publish. A www prefix that means nothing simply fails to
	// resolve, which costs one bounded fetch; a missing one costs the whole
	// site read.
	host := parsed.Host
	var hosts []string
	if after, found := strings.CutPrefix(host, "www."); found {
		hosts = []string{host, after}
	} else {
		hosts = []string{host, "www." + host}
	}

	var out []string
	for _, scheme := range []string{schemeHTTPS, schemeHTTP} {
		for _, h := range hosts {
			candidate := *parsed
			candidate.Scheme = scheme
			candidate.Host = h
			if spelling := candidate.String(); spelling != seedURL {
				out = append(out, spelling)
			}
		}
	}
	return out
}

// applyCrawlDelay slows the rest of the crawl to the rate the site publishes.
//
// It runs after the seed answered, which is exactly when the host's robots.txt
// has been fetched and cached — asking earlier would either force a second
// network round trip or read an empty cache and conclude the site asked for
// nothing. Every later page of this crawl goes through the same pacer, so one
// call here governs the whole read.
func (c *siteCrawler) applyCrawlDelay(pacer crawlPacer, answeredURL string) {
	if delay, asked := c.fetch.CrawlDelay(answeredURL); asked {
		pacer.SlowTo(delay)
	}
}
