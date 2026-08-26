// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a redirect means to the crawl. The fetch follows redirects, so the
// requested URL and the URL that served the body can be two different sites.
// The SEED's redirect is the site's own answer about where it lives — the
// crawl boundary, probe origin, pacing and evidence all move with it. A LATER
// page's redirect never moves anything: off-site destinations are discarded
// unread, exactly like off-site links.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestCrawlFollowsTheSeedRedirectOntoAnotherDomain(t *testing.T) {
	// The imported domain answers 301 onto a different registrable domain —
	// acme.co forwarding to www.acme.vn. Every useful link on the landing page
	// names the destination; a crawl still bounded by the requested domain
	// would reject them all and read one page.
	const requested = "https://acme.co"
	const answering = "https://www.acme.vn"
	site := &fakeSite{pages: map[string]fakeSitePage{
		requested: {
			finalURL: answering,
			text:     readable("Welcome to Acme."),
			links:    []string{answering + "/about"},
		},
		answering + "/about": {text: readable("About Acme.")},
	}}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), requested)
	if err != nil {
		t.Fatal(err)
	}
	if crawl.SeedURL != answering {
		t.Fatalf("SeedURL = %q, want the answering %q", crawl.SeedURL, answering)
	}
	if crawl.Pages[0].URL != answering {
		t.Fatalf("seed evidence URL = %q, want the URL that served it, %q", crawl.Pages[0].URL, answering)
	}
	var aboutRead bool
	for _, page := range crawl.Pages {
		if page.URL == answering+"/about" {
			aboutRead = true
		}
	}
	if !aboutRead {
		t.Fatalf("the destination's own link was not crawled: %v", crawl.Pages)
	}
	for _, skip := range crawl.Skipped {
		if skip.URL == answering+"/about" {
			t.Fatalf("a destination-site link was skipped as %s", skip.Reason)
		}
	}
	// Discovery probes ask the site that answered, not the forwarder.
	if !slicesContains(site.fetched, answering+"/impressum") {
		t.Errorf("probes did not move to the answering origin: %v", site.fetched)
	}
	if slicesContains(site.fetched, requested+"/impressum") {
		t.Errorf("a probe still asked the forwarding host: %v", site.fetched)
	}
}

func TestCrawlSeedApexToWWWRedirectReadsThePageOnce(t *testing.T) {
	// The ordinary case: the apex forwards to www on the same site. The crawl
	// carries the www identity, and a nav link back to the apex spelling is
	// the page already read, not a second fetch.
	const requested = "https://acme.example"
	const answering = "https://www.acme.example"
	site := &fakeSite{pages: map[string]fakeSitePage{
		requested: {
			finalURL: answering,
			text:     readable("Welcome home."),
			links:    []string{requested, answering + "/team"},
		},
		answering + "/team": {text: readable("Our team.")},
	}}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), requested)
	if err != nil {
		t.Fatal(err)
	}
	if crawl.SeedURL != answering {
		t.Fatalf("SeedURL = %q, want %q", crawl.SeedURL, answering)
	}
	if site.pageCalls[requested] != 1 {
		t.Fatalf("the apex spelling was fetched %d times, want once", site.pageCalls[requested])
	}
	var teamRead bool
	for _, page := range crawl.Pages {
		if page.URL == answering+"/team" {
			teamRead = true
		}
	}
	if !teamRead {
		t.Fatalf("a same-site link was not crawled after the seed redirect: %v", crawl.Pages)
	}
}

func TestCrawlDiscardsALaterPageThatRedirectsOffSite(t *testing.T) {
	// A page on the site answering from ANOTHER site is the boundary crossing
	// the off-domain gate exists for, one fetch later: the body — and every
	// link in it — must be discarded, and only the seed's redirect may ever
	// move the boundary.
	partner := seedURL + "/partner"
	site := &fakeSite{pages: seedOnly("/partner")}
	site.pages[partner] = fakeSitePage{
		finalURL: "https://evil.example/landing",
		text:     readable("Content served by a different site."),
		links:    []string{"https://evil.example/deeper"},
	}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range crawl.Pages {
		if page.URL == partner || page.URL == "https://evil.example/landing" {
			t.Fatalf("an off-site-redirecting page was committed as %q", page.URL)
		}
	}
	want := crawlSkip{URL: partner, Reason: crmcontracts.SiteReadSkipReasonOffDomain}
	var recorded bool
	for _, skip := range crawl.Skipped {
		if skip == want {
			recorded = true
		}
	}
	if !recorded {
		t.Fatalf("the off-site redirect was not recorded as an off_domain skip: %v", crawl.Skipped)
	}
	if slicesContains(site.fetched, "https://evil.example/deeper") {
		t.Fatal("a link from the discarded off-site body was fetched")
	}
}

func TestCrawlPacesToTheAnsweringHostsCrawlDelay(t *testing.T) {
	// Politeness follows the site that serves, not the host that forwards:
	// the destination's robots.txt is the one whose Crawl-delay binds.
	const requested = "https://acme.co"
	const answering = "https://www.acme.vn"
	site := &fakeSite{
		pages: map[string]fakeSitePage{
			requested: {finalURL: answering, text: readable("Welcome.")},
		},
		crawlDelays: map[string]time.Duration{answering: 2 * time.Second},
	}
	var slowed time.Duration
	crawler := testSiteCrawler(site)
	crawler.newPacer = func() crawlPacer { return instantPacer{slowedTo: &slowed} }

	if _, err := crawler.Crawl(context.Background(), requested); err != nil {
		t.Fatal(err)
	}
	if slowed != 2*time.Second {
		t.Fatalf("pacer slowed to %s, want the answering host's 2s", slowed)
	}
}

func TestCrawlEvidenceNamesTheURLThatServedThePage(t *testing.T) {
	// A same-site redirect (an old path forwarding to the page's real home):
	// the committed evidence URL is the destination, and that destination is
	// marked read so another route to it costs no second fetch. Both paths
	// classify as "other", so nav insertion order — the old path first —
	// decides which fetch happens.
	oldPath := seedURL + "/legacy-page"
	newPath := seedURL + "/current-page"
	site := &fakeSite{pages: seedOnly("/legacy-page", "/current-page")}
	pageText := readable("The page that moved.")
	site.pages[oldPath] = fakeSitePage{finalURL: newPath, text: pageText}
	site.pages[newPath] = fakeSitePage{text: pageText}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	var served, requestedSpelling bool
	for _, page := range crawl.Pages {
		if page.URL == newPath {
			served = true
		}
		if page.URL == oldPath {
			requestedSpelling = true
		}
	}
	if !served || requestedSpelling {
		t.Fatalf("evidence should name only the serving URL %q: %v", newPath, crawl.Pages)
	}
	if site.pageCalls[newPath] != 0 {
		t.Fatalf("the redirect target was fetched again %d times although its content was already read", site.pageCalls[newPath])
	}
}

func TestCrawlClassifiesARedirectedPageByItsDestination(t *testing.T) {
	// A guessed /impressum that forwards to a careers page is not a legal
	// notice: the committed kind follows the URL that served the body, and
	// the unsatisfied probe kind stays open, so the next legal-page guess is
	// still tried and the entity census still gets its page.
	site := &fakeSite{pages: seedOnly()}
	site.pages[seedURL+"/impressum"] = fakeSitePage{finalURL: seedURL + "/jobs", text: readable("Open roles at Acme.")}
	site.pages[seedURL+"/imprint"] = fakeSitePage{text: readable("Acme GmbH, HRB 12345.")}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]crmcontracts.SiteReadPageKind{}
	for _, page := range crawl.Pages {
		kinds[page.URL] = page.Kind
	}
	if kinds[seedURL+"/jobs"] != crmcontracts.SiteReadPageKindOther {
		t.Fatalf("the careers destination was committed as %q, want other: %v", kinds[seedURL+"/jobs"], kinds)
	}
	if kinds[seedURL+"/imprint"] != crmcontracts.SiteReadPageKindImpressum {
		t.Fatalf("the impressum kind was closed by a redirected probe; pages = %v", kinds)
	}
}
