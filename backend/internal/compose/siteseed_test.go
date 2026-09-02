// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestSeedFallbacksWalksTheSpellingsOfTheSameSite(t *testing.T) {
	got := seedFallbacks("https://acme.com")
	want := []string{"https://www.acme.com", "http://acme.com", "http://www.acme.com"}
	if !slices.Equal(got, want) {
		t.Errorf("seedFallbacks = %v, want %v", got, want)
	}
}

func TestSeedFallbacksOffersTheApexWhenTheSeedCarriesWWW(t *testing.T) {
	got := seedFallbacks("https://www.acme.com")
	want := []string{"https://acme.com", "http://www.acme.com", "http://acme.com"}
	if !slices.Equal(got, want) {
		t.Errorf("seedFallbacks = %v, want %v", got, want)
	}
}

// A label is never STRIPPED except the exact `www` prefix: dropping any other
// one points at a host that may not be this company at all.
func TestSeedFallbacksNeverStripsALabelThatIsNotWWW(t *testing.T) {
	for _, seed := range []string{"https://careers.acme.com", "https://shop.acme.co.uk"} {
		for _, candidate := range seedFallbacks(seed) {
			for _, forbidden := range []string{
				"https://acme.com", "https://www.acme.com",
				"https://acme.co.uk", "https://www.acme.co.uk",
			} {
				if candidate == forbidden {
					t.Errorf("seed %q produced %q — a different host", seed, candidate)
				}
			}
		}
	}
}

// A multi-label public suffix is not a subdomain. Counting dots called
// acme.co.uk one and skipped its www spelling, which is where a good share of
// UK and German companies actually publish.
func TestSeedFallbacksOffersWWWForAMultiLabelSuffix(t *testing.T) {
	got := seedFallbacks("https://acme.co.uk")
	if !slices.Contains(got, "https://www.acme.co.uk") {
		t.Errorf("seedFallbacks = %v, want the www spelling offered", got)
	}
}

func TestSeedFallbacksKeepsThePathAndNeverRepeatsTheSeed(t *testing.T) {
	for _, candidate := range seedFallbacks("https://acme.com/de") {
		if candidate == "https://acme.com/de" {
			t.Fatal("the seed itself must never be offered as its own fallback")
		}
		if !slices.Contains([]string{
			"https://www.acme.com/de", "http://acme.com/de", "http://www.acme.com/de",
		}, candidate) {
			t.Errorf("candidate %q dropped or changed the path", candidate)
		}
	}
}

func TestSeedFallbacksRefusesWhatItCannotParseOrShouldNotFetch(t *testing.T) {
	for _, seed := range []string{"", "://nope", "ftp://acme.com", "file:///etc/passwd", "not a url"} {
		if got := seedFallbacks(seed); len(got) != 0 {
			t.Errorf("seedFallbacks(%q) = %v, want none", seed, got)
		}
	}
}

// The defect this fixes: a landing page that only forwards to the real site
// read as a page with nothing on it, so the domain triage called the site
// parked and the company was never created.
func TestASeedThatOnlyForwardsIsFollowedToTheRealLandingPage(t *testing.T) {
	site := &fakeSite{pages: map[string]fakeSitePage{
		"https://anwr-group.com":    {refresh: "https://anwr-group.com/de"},
		"https://anwr-group.com/de": {text: "ANWR Group is a retail cooperative."},
	}}
	page, err := newSiteCrawler(site, CrawlCaps{}).ReadSeed(context.Background(), "https://anwr-group.com")
	if err != nil {
		t.Fatalf("reading the seed: %v", err)
	}
	if want := "https://anwr-group.com/de"; page.URL != want {
		t.Errorf("landing page URL = %q, want %q", page.URL, want)
	}
	if page.Text == "" {
		t.Error("the landing page carries no text — the triage will call this site parked")
	}
}

// One hop, never a chain. A site can write as many trampolines as it likes,
// and each one the crawler follows is a fetch it was told to make by the page
// it just read.
func TestAChainOfForwardingPagesIsFollowedOnlyOnce(t *testing.T) {
	site := &fakeSite{pages: map[string]fakeSitePage{
		"https://acme.example":     {refresh: "https://acme.example/one"},
		"https://acme.example/one": {refresh: "https://acme.example/two"},
		"https://acme.example/two": {text: "the real site"},
	}}
	page, err := newSiteCrawler(site, CrawlCaps{}).ReadSeed(context.Background(), "https://acme.example")
	if err != nil {
		t.Fatalf("reading the seed: %v", err)
	}
	if want := "https://acme.example/one"; page.URL != want {
		t.Errorf("landing page URL = %q, want %q — exactly one hop", page.URL, want)
	}
	if slices.Contains(site.fetched, "https://acme.example/two") {
		t.Errorf("the crawler followed a second hop: %v", site.fetched)
	}
}

// Two pages forwarding to each other would loop forever if the follow were
// not capped at one, and a site can publish that by accident.
func TestTwoPagesForwardingToEachOtherTerminate(t *testing.T) {
	site := &fakeSite{pages: map[string]fakeSitePage{
		"https://acme.example":    {refresh: "https://acme.example/de"},
		"https://acme.example/de": {refresh: "https://acme.example"},
	}}
	if _, err := newSiteCrawler(site, CrawlCaps{}).ReadSeed(context.Background(), "https://acme.example"); err != nil {
		t.Fatalf("reading the seed: %v", err)
	}
	if len(site.fetched) > 2 {
		t.Errorf("fetched %v — a forwarding cycle must cost at most one extra fetch", site.fetched)
	}
}

// When the forwarding target does not answer, the triage must be left with the
// evidence it actually had rather than an error: the empty shell is a real
// read, and the site may still be judged from what else the crawl finds.
func TestAForwardingTargetThatFailsLeavesTheOriginalPage(t *testing.T) {
	site := &fakeSite{
		pages: map[string]fakeSitePage{
			"https://acme.example": {refresh: "https://acme.example/de"},
		},
		pageErrors: map[string][]error{
			"https://acme.example/de": {errors.New("the forwarding target answered 500")},
		},
	}
	page, err := newSiteCrawler(site, CrawlCaps{}).ReadSeed(context.Background(), "https://acme.example")
	if err != nil {
		t.Fatalf("a failed follow must not fail the seed read: %v", err)
	}
	if want := "https://acme.example"; page.URL != want {
		t.Errorf("landing page URL = %q, want the page that did answer, %q", page.URL, want)
	}
}

// A site reached through the www/scheme ladder gets the same follow: the two
// shapes co-occur constantly, because the host that forwards by markup is
// usually also the one that only answers on www.
func TestASeedReachedThroughTheFallbackLadderIsAlsoFollowed(t *testing.T) {
	site := &fakeSite{pages: map[string]fakeSitePage{
		"https://www.acme.example":    {refresh: "https://www.acme.example/de"},
		"https://www.acme.example/de": {text: "the real site"},
	}}
	page, err := newSiteCrawler(site, CrawlCaps{}).ReadSeed(context.Background(), "https://acme.example")
	if err != nil {
		t.Fatalf("reading the seed: %v", err)
	}
	if want := "https://www.acme.example/de"; page.URL != want {
		t.Errorf("landing page URL = %q, want %q", page.URL, want)
	}
	if page.Text == "" {
		t.Error("the landing page carries no text after the ladder plus the follow")
	}
}
