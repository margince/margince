// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/webread"
)

func pageWith(url string, fingerprint webread.Fingerprint) crawlPage {
	fingerprint.URL = url
	return crawlPage{URL: url, Fingerprint: fingerprint}
}

// The reason this lane exists: a shop system announces itself on the page that
// runs it, and the homepage never mentions it. A homepage-only read reports
// that this company runs nothing but nginx.
func TestATechnologyIsFoundOnThePageThatRunsIt(t *testing.T) {
	t.Parallel()
	got, err := technologiesAcross([]crawlPage{
		pageWith("https://example.de/", webread.Fingerprint{
			Headers: http.Header{"Server": []string{"nginx"}},
		}),
		pageWith("https://example.de/shop", webread.Fingerprint{
			Headers:     http.Header{"Server": []string{"nginx"}},
			CookieNames: []string{"sw-states"},
		}),
	})
	if err != nil {
		t.Fatalf("matching the crawl: %v", err)
	}
	if !observedIn(got, people.FactTechnology, "shopware") {
		t.Errorf("the shop system on /shop was not read; read %v", got)
	}
}

// Evidence has to name the page that PROVED the technology, or a reader asking
// "how do you know?" is sent to a page that never mentioned it.
func TestTheEvidenceCitesTheProvingPage(t *testing.T) {
	t.Parallel()
	got, err := technologiesAcross([]crawlPage{
		pageWith("https://example.de/", webread.Fingerprint{}),
		pageWith("https://example.de/karriere", webread.Fingerprint{
			CookieNames: []string{"fe_typo_user"},
		}),
	})
	if err != nil {
		t.Fatalf("matching the crawl: %v", err)
	}
	for _, observation := range got {
		if observation.ValueKey == "typo3" {
			if observation.SourceURL != "https://example.de/karriere" {
				t.Errorf("cited %q, want the page that proved it", observation.SourceURL)
			}
			return
		}
	}
	t.Errorf("typo3 was not read at all; read %v", got)
}

// One row per technology, however many pages carry the same marker — a site
// serving nginx on thirty pages states one fact about itself.
func TestATechnologyOnEveryPageIsOneObservation(t *testing.T) {
	t.Parallel()
	same := webread.Fingerprint{Headers: http.Header{"Server": []string{"nginx"}}}
	got, err := technologiesAcross([]crawlPage{
		pageWith("https://example.de/", same),
		pageWith("https://example.de/about", same),
		pageWith("https://example.de/contact", same),
	})
	if err != nil {
		t.Fatalf("matching the crawl: %v", err)
	}
	count := 0
	for _, observation := range got {
		if observation.ValueKey == "nginx" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("nginx observed %d times, want 1: %v", count, got)
	}
}

// A page the crawl recorded without a fetched response carries a zero
// fingerprint. It must be skipped rather than matched as an empty page, which
// is what the certification fixtures build.
func TestAPageWithNoFetchedResponseIsSkipped(t *testing.T) {
	t.Parallel()
	got, err := technologiesAcross([]crawlPage{{URL: "https://example.de/", Text: "hello"}})
	if err != nil {
		t.Fatalf("matching the crawl: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("read %v from a page that was never fetched", got)
	}
}

func observedIn(in []people.TechnicalObservation, field, key string) bool {
	for _, observation := range in {
		if observation.Field == field && observation.ValueKey == key {
			return true
		}
	}
	return false
}
