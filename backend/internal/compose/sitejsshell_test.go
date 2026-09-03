// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A client-rendered site sends a loader, not its words. Judging that "no
// readable text" puts a real company on file as an empty address, with no model
// call made and nothing later to reopen it — the same failure the meta-refresh
// follow exists to prevent, arriving by another route.
//
// The worker is the zero value: it carries no brain, so a verdict that needed a
// model call would panic rather than pass quietly.
func TestAJavaScriptShellIsNotSettledAsParked(t *testing.T) {
	shell := crawlPage{Text: "", ExternalScripts: 3, ModuleScripts: 1}
	verdict, err := (&siteDeepReadWorker{}).classifySeed(context.Background(), shell)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if verdict.Aborts() {
		t.Errorf("verdict %+v settles the domain — a shell this reader cannot render is an unread page, not an empty one", verdict)
	}
}

// The guard's other half. A page with nothing on it and nothing to build it
// with is genuinely empty, and that answer still costs no model call.
func TestAnEmptyPageWithNoScriptsIsStillParked(t *testing.T) {
	empty := crawlPage{Text: "", ExternalScripts: 0}
	verdict, err := (&siteDeepReadWorker{}).classifySeed(context.Background(), empty)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if !verdict.Aborts() {
		t.Errorf("verdict %+v does not settle a genuinely empty landing page", verdict)
	}
}

// A parked domain carries analytics and a registrar's own script as readily as
// a real site does. Counting ANY external script would let a squatter's
// placeholder escape the parked verdict and become a pending, retried,
// never-evidenced read — so the shell test asks for a MODULE, which is how
// every current bundler emits an application entry point.
func TestAParkedPageWithAnalyticsIsStillParked(t *testing.T) {
	parked := crawlPage{Text: "", ExternalScripts: 2, ModuleScripts: 0}
	verdict, err := (&siteDeepReadWorker{}).classifySeed(context.Background(), parked)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if !verdict.Aborts() {
		t.Errorf("verdict %+v leaves a parked domain open — a tag manager is not an application", verdict)
	}
}

// A shell that DID declare what it is about has prose to judge, so it reaches
// the classifier — with that declaration in the prompt, which is the whole
// point of harvesting it.
func TestAShellThatDeclaresItselfReachesTheClassifierWithIt(t *testing.T) {
	declared := crawlPage{
		URL: seedURL, Text: "", ExternalScripts: 2, ModuleScripts: 1,
		HeadText: []string{"We build robots for warehouses."},
	}
	// It is NOT settled deterministically: the empty-text shortcut steps aside
	// for a page that said something about itself.
	if declared.prose() == "" {
		t.Fatal("a declared shell has prose to judge")
	}
	req := triageRequest(declared, "en")
	if len(req.Messages) == 0 {
		t.Fatal("the triage request carries no message")
	}
	if !strings.Contains(req.Messages[0].Content, "We build robots for warehouses.") {
		t.Fatalf("the declaration never reached the triage prompt:\n%s", req.Messages[0].Content)
	}
}

// What the profile lane is shown: the page's own claim about itself, ahead of
// its body text, inside the same budget every other passage is charged.
func TestTheProfileCorpusCarriesWhatTheHeadDeclared(t *testing.T) {
	page := crawlPage{
		URL: seedURL, Kind: crmcontracts.SiteReadPageKindHome,
		Text:     "Body words.",
		HeadText: []string{"A description."},
	}
	excerpts := profileExcerptPages([]crawlPage{page})
	if len(excerpts) == 0 {
		t.Fatal("the home page did not reach the profile corpus")
	}
	if !strings.Contains(excerpts[0].Text, "A description.") {
		t.Fatalf("excerpt = %q, want the head declaration in the corpus", excerpts[0].Text)
	}
	// The crawl's own record of the page is untouched: evidence snippets from
	// other lanes are matched against StripTags' output.
	if page.Text != "Body words." {
		t.Fatalf("Text = %q — building the corpus must not rewrite the page", page.Text)
	}
}

// A page that declared nothing reads exactly as it did before.
func TestAPageWithNoHeadDeclarationReadsAsItsTextAlone(t *testing.T) {
	page := crawlPage{Text: "Body words."}
	if got := page.prose(); got != "Body words." {
		t.Fatalf("prose() = %q, want the text unchanged", got)
	}
}

// The dossier says what kind of read this was, so a thin result is not
// mistaken for a thin company.
func TestTheDossierSaysWhenOnlyAShellCouldBeRead(t *testing.T) {
	shell := []crawlPage{{Kind: crmcontracts.SiteReadPageKindHome, Text: "", ExternalScripts: 4, ModuleScripts: 1}}
	warnings := readWarnings("", nil, seedIsJSShell(shell))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "browser") {
		t.Fatalf("warnings = %v, want one naming that the site builds its pages in the browser", warnings)
	}
	ordinary := []crawlPage{{Kind: crmcontracts.SiteReadPageKindHome, Text: strings.Repeat("word ", 40)}}
	if got := readWarnings("", nil, seedIsJSShell(ordinary)); len(got) != 0 {
		t.Fatalf("warnings = %v, want none for a site that served its words", got)
	}
}

// The same catch-all, serving a SHORT document — which is what a
// client-rendered site actually sends. The duplicate was recognised only after
// the text floor had already reported it, so one loader shell became
// thirty-one `unreadable` skips and the report read as a site that could not be
// read rather than one page seen many times.
func TestADuplicateShellIsSilentEvenThoughItIsShort(t *testing.T) {
	shell := fakeSitePage{text: "Acme", scripts: 2, modules: 1}
	site := &fakeSite{pages: map[string]fakeSitePage{
		seedURL: {text: readable("Welcome to Acme."), links: []string{
			seedURL + "/about", seedURL + "/team", seedURL + "/kontakt",
		}},
	}}
	// The catch-all: every path a probe guesses at answers 200 with the same
	// loader, which is what a client-rendered site does and why the probes all
	// come back looking like separate unreadable pages.
	for _, probe := range wellKnownProbes {
		site.pages[seedURL+probe.path] = shell
	}
	for _, path := range []string{"/about", "/team", "/kontakt"} {
		site.pages[seedURL+path] = shell
	}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	var reported []string
	for _, skip := range crawl.Skipped {
		if skip.Reason == crmcontracts.SiteReadSkipReasonUnreadable {
			reported = append(reported, skip.URL)
		}
	}
	// One document, said once. The first shell is honest degradation and
	// belongs in the report; the repeats are the same fact again, and it was
	// repeating them thirty-one times that made a readable site look unread.
	if len(reported) != 1 {
		t.Fatalf("unreadable skips = %d, want 1 — one repeated document is one fact: %v",
			len(reported), reported)
	}
	// And it is the FIRST one walked, not whichever happened to land last:
	// probes run in wellKnownProbes order ahead of the seed's own links.
	if want := seedURL + wellKnownProbes[0].path; reported[0] != want {
		t.Fatalf("the reported shell was %q, want the first one walked %q", reported[0], want)
	}
}

// The seed's own body is in the dedupe from the start, so a catch-all that
// answers every path with the LANDING page reports nothing at all — the shape
// TestCrawlSkipsADuplicateTextPageSilently covers for a long page, here for a
// short one, which is the half that used to fall through the text floor.
func TestACatchAllServingTheSeedItselfIsSilent(t *testing.T) {
	shell := fakeSitePage{text: "Acme", scripts: 1, modules: 1}
	site := &fakeSite{pages: map[string]fakeSitePage{seedURL: shell}}
	for _, probe := range wellKnownProbes {
		site.pages[seedURL+probe.path] = shell
	}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, skip := range crawl.Skipped {
		if skip.Reason == crmcontracts.SiteReadSkipReasonUnreadable {
			t.Fatalf("the seed's own body was reported as an unreadable page at %s", skip.URL)
		}
	}
}

// Two routes can share a body and still say different things about themselves.
// Once a page's own declaration became evidence the profile lane reads, keying
// the dedupe on body text alone would drop the second silently — taking a
// description no other page carries with it.
func TestTwoRoutesWithDifferentDeclarationsAreTwoDocuments(t *testing.T) {
	body := readable("Shared body.")
	site := &fakeSite{pages: map[string]fakeSitePage{
		seedURL: {
			text: body, headText: []string{"The home page."},
			links: []string{seedURL + "/services"},
		},
		seedURL + "/services": {text: body, headText: []string{"What we build for clients."}},
	}}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range crawl.Pages {
		if page.URL == seedURL+"/services" {
			return
		}
	}
	t.Fatalf("the second declaration vanished as a duplicate: %+v", crawl.Pages)
}

// A short page nobody has seen before is still honest degradation, and still
// belongs in the report.
func TestAShortPageSeenOnceIsStillReportedUnreadable(t *testing.T) {
	site := &fakeSite{pages: seedOnly("/about")}
	site.pages[seedURL+"/about"] = fakeSitePage{text: "Thin", scripts: 1}

	crawl, err := testSiteCrawler(site).Crawl(context.Background(), seedURL)
	if err != nil {
		t.Fatal(err)
	}
	for _, skip := range crawl.Skipped {
		if skip.URL == seedURL+"/about" && skip.Reason == crmcontracts.SiteReadSkipReasonUnreadable {
			return
		}
	}
	t.Fatalf("a unique unreadable page vanished from the report: %+v", crawl.Skipped)
}
