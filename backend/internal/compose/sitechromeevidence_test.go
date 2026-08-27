// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A finding cited to the site's own navigation is not a finding about the
// business.
//
// Both cases here come from one live read of a real multi-entity site: a legal
// entity whose evidence was the flattened mega-menu, and language-switcher
// labels returned as `language` facts scored 100. The menu names every
// candidate equally, so it distinguishes none of them; the switcher's labels
// are true about the WEBSITE and were being pre-selected into the company
// context as facts about the company.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
)

// theMenu is the shared opening a real site carries on every page: long enough
// to qualify as chrome, and carrying exactly the two things that came back as
// findings — the entity names and the language switcher.
const theMenu = "Imprint | Gradion | Gradion Solutions Products Industries About English Contact Us " +
	"Solutions Products Industries About English Deutsch Tiếng Việt ไทย العربية 日本語 Contact Us " +
	"Imprint Careers Newsroom Partners Investors Legal Privacy Terms Cookies Sitemap Accessibility " +
	"Solutions Products Industries About Contact Us Imprint Careers Newsroom Partners Investors"

// chromedCrawl is a crawl whose pages all open with the menu and then say
// something of their own — the shape the boilerplate measurement is built for.
func chromedCrawl(own ...string) []crawlPage {
	pages := make([]crawlPage, 0, len(own))
	for i, text := range own {
		pages = append(pages, crawlPage{
			URL:  "https://example.test/p" + string(rune('a'+i)),
			Text: theMenu + " " + text,
		})
	}
	return pages
}

func TestAFindingCitedOnlyToTheMenuIsDropped(t *testing.T) {
	pages := chromedCrawl(
		"Gradion Solutions GmbH is registered in Berlin under HRB 12345. The company was founded in 2014 and files its annual accounts with the Amtsgericht Charlottenburg each spring. The registered office has not moved since incorporation, and the managing directors are listed on this page together with the supervisory board. Correspondence about the register entry should go to the address printed below rather than to the sales team, who cannot answer it.",
		"We build inventory software for wholesalers across Europe and Asia, and have done so since the company was founded. Our customers run distribution networks of every size, from a single warehouse to a continent of them, and the platform is the same one either way. What changes is how much of it a customer switches on, which is a decision they make gradually rather than at the start, and which our pricing is deliberately built to allow.",
		"Our team of forty works from Berlin, Hanoi and Bangkok, and we hire in all three cities. Most of the engineering happens in Berlin; support follows the sun across the others, so a customer in any timezone reaches somebody who can act rather than somebody who can log it. We have kept that arrangement since the second year, and it is the reason our response times do not have a nightly cliff in them the way our competitors' do.",
		"Contact our sales team at hello@example.test for a demonstration of the platform. We will walk you through the catalogue, the replenishment engine and the reporting surface, using your own data if you can share an export and ours if you would rather not. A demonstration takes about an hour and there is no obligation attached to it. If you would prefer to read first, the documentation is public and the pricing page states every number.",
		"Careers at Gradion: we are hiring engineers and account managers in every office we run. Applications are read by the team you would join rather than by a recruiting funnel, and we answer every one of them whether or not we take it further. The interview is a conversation about work you have actually done, not a whiteboard exercise, and we pay for any take-home we ask for. Our salary bands are published on the same page as the roles.",
	)
	results := []pageFactsResult{{
		url: pages[0].URL,
		facts: []people.DeepReadFact{
			// The switcher's own labels, cited to the block that carries them.
			{
				Category: "language", Field: "language", Value: "Tiếng Việt",
				EvidenceSnippet: "English Deutsch Tiếng Việt ไทย العربية 日本語", Confidence: 1,
			},
			// A fact the page actually states, cited to its own prose.
			{
				Category: "registration", Field: "register_number", Value: "HRB 12345",
				EvidenceSnippet: "Gradion Solutions GmbH is registered in Berlin under HRB 12345.", Confidence: 1,
			},
		},
		entities: []corpusLegalEntity{
			{Name: "Gradion Solutions", EvidenceSnippet: "Imprint | Gradion | Gradion Solutions Products Industries About"},
			{Name: "Gradion Solutions GmbH", EvidenceSnippet: "Gradion Solutions GmbH is registered in Berlin under HRB 12345."},
		},
	}}

	kept, dropped := suppressChromeEvidence(pages, results)

	if len(kept) != 1 {
		t.Fatalf("the suppression returned %d results for 1 page", len(kept))
	}
	if len(kept[0].facts) != 1 || kept[0].facts[0].Field != "register_number" {
		t.Errorf("facts kept = %+v, want only the one its page states — a switcher label is true "+
			"about the website and was being offered as a fact about the company", kept[0].facts)
	}
	if len(kept[0].entities) != 1 || kept[0].entities[0].Name != "Gradion Solutions GmbH" {
		t.Errorf("entities kept = %+v, want only the one named in prose — the menu names every "+
			"candidate equally, so it distinguishes none of them", kept[0].entities)
	}
	if len(dropped) != 2 {
		t.Fatalf("%d finding(s) reported as dropped, want 2 — a finding that disappears without "+
			"saying so is worse than one that stays", len(dropped))
	}
	for _, d := range dropped {
		if d.Lane != chromeEvidenceLane {
			t.Errorf("a drop was filed under lane %q, want %q", d.Lane, chromeEvidenceLane)
		}
		if !strings.Contains(d.Reason, "navigation") {
			t.Errorf("the drop reason %q does not say what was wrong with the evidence", d.Reason)
		}
	}
}

// A crawl this cannot measure keeps everything. A missing finding is harder to
// notice than a noisy one, so the failure direction is deliberate.
func TestACrawlWithNoMeasurableChromeKeepsEveryFinding(t *testing.T) {
	pages := []crawlPage{
		{URL: "https://example.test/a", Text: "We build inventory software for wholesalers."},
		{URL: "https://example.test/b", Text: "Our team of forty works from Berlin and Hanoi."},
	}
	results := []pageFactsResult{{
		url:   pages[0].URL,
		facts: []people.DeepReadFact{{Field: "language", Value: "Tiếng Việt", EvidenceSnippet: "Tiếng Việt"}},
	}}

	kept, dropped := suppressChromeEvidence(pages, results)

	if len(kept[0].facts) != 1 {
		t.Errorf("a finding was dropped on a crawl with no measurable chrome: %+v", kept[0].facts)
	}
	if len(dropped) != 0 {
		t.Errorf("%d drop(s) reported where nothing could be measured", len(dropped))
	}
}

// A finding with no evidence at all is the citation gate's business. Answering
// it here would tell a reader the wrong reason.
func TestAFindingWithNoEvidenceIsNotChrome(t *testing.T) {
	pages := chromedCrawl(
		"We build inventory software for wholesalers across Europe and Asia, and have done so since the company was founded. Our customers run distribution networks of every size, from a single warehouse to a continent of them, and the platform is the same one either way. What changes is how much of it a customer switches on, which is a decision they make gradually rather than at the start, and which our pricing is deliberately built to allow.",
		"Our team of forty works from Berlin, Hanoi and Bangkok, and we hire in all three cities. Most of the engineering happens in Berlin; support follows the sun across the others, so a customer in any timezone reaches somebody who can act rather than somebody who can log it. We have kept that arrangement since the second year, and it is the reason our response times do not have a nightly cliff in them the way our competitors' do.",
		"Contact our sales team at hello@example.test for a demonstration of the platform. We will walk you through the catalogue, the replenishment engine and the reporting surface, using your own data if you can share an export and ours if you would rather not. A demonstration takes about an hour and there is no obligation attached to it. If you would prefer to read first, the documentation is public and the pricing page states every number.",
		"Careers at Gradion: we are hiring engineers and account managers in every office we run. Applications are read by the team you would join rather than by a recruiting funnel, and we answer every one of them whether or not we take it further. The interview is a conversation about work you have actually done, not a whiteboard exercise, and we pay for any take-home we ask for. Our salary bands are published on the same page as the roles.",
		"Gradion Solutions GmbH is registered in Berlin under HRB 12345. The company was founded in 2014 and files its annual accounts with the Amtsgericht Charlottenburg each spring. The registered office has not moved since incorporation, and the managing directors are listed on this page together with the supervisory board. Correspondence about the register entry should go to the address printed below rather than to the sales team, who cannot answer it.",
	)
	results := []pageFactsResult{{
		url:   pages[0].URL,
		facts: []people.DeepReadFact{{Field: "industry", Value: "software", EvidenceSnippet: ""}},
	}}

	kept, dropped := suppressChromeEvidence(pages, results)

	if len(kept[0].facts) != 1 {
		t.Error("a finding with no evidence was dropped as chrome — it has a reason of its own, " +
			"and this is not it")
	}
	if len(dropped) != 0 {
		t.Errorf("%d drop(s) reported for a finding this lane does not judge", len(dropped))
	}
}
