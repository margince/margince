// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// serveHTML answers every path with one document, the way a client-rendered
// site's catch-all does.
func serveHTML(t *testing.T, doc string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		//craft:ignore swallowed-errors httptest handler write; a failed write fails the test through the assertions below
		_, _ = w.Write([]byte(doc))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// The shape that produced the bug: a Vite build's index.html, whose whole
// server-sent prose is the <title> and the head declarations. What the company
// does is stated in the description, and it was being thrown away.
const shellHTML = `<!doctype html><html lang="en"><head>
<title>Erler Ventures | The Operating System for Founders</title>
<meta name="description" content="90% of startups fail. Yours doesn't have to. The systematic operating system for founders and scaling companies." />
<meta property="og:title" content="Erler Ventures | The Operating System for Founders" />
<meta property="og:description" content="Systematic execution from Day 1 to profitable scale." />
<script type="module" crossorigin src="/assets/index-DyqdXLtN.js"></script>
</head><body><div id="root"></div></body></html>`

func TestAPageCarriesWhatItsHeadSaysItIsAbout(t *testing.T) {
	srv := serveHTML(t, shellHTML)
	page, err := testFetcher().FetchPage(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"90% of startups fail. Yours doesn't have to. The systematic operating system for founders and scaling companies.",
		"Erler Ventures | The Operating System for Founders",
		"Systematic execution from Day 1 to profitable scale.",
	}
	if !reflect.DeepEqual(page.HeadText, want) {
		t.Fatalf("Page.HeadText = %q,\nwant %q", page.HeadText, want)
	}
	// The text contract is what stored evidence snippets are matched against,
	// so the head prose must NOT have been folded into it.
	if page.Text != StripTags(shellHTML) {
		t.Fatalf("Page.Text = %q diverged from StripTags", page.Text)
	}
	if page.ExternalScripts != 1 {
		t.Fatalf("Page.ExternalScripts = %d, want 1 — the module bundle", page.ExternalScripts)
	}
}

// A page repeating a declaration offers no basis to rank the repeats, and a
// title echoed as og:title is one sentence, not two.
func TestAHeadDeclarationIsHarvestedOnce(t *testing.T) {
	doc := `<html><head>
<meta name="description" content="The first one." />
<meta name="description" content="A second, later one." />
<meta property="og:title" content="The first one." />
</head><body>Words enough to read.</body></html>`
	page, err := testFetcher().FetchPage(context.Background(), serveHTML(t, doc).URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"The first one."}; !reflect.DeepEqual(page.HeadText, want) {
		t.Fatalf("Page.HeadText = %q, want %q — first declaration wins, duplicates drop", page.HeadText, want)
	}
}

// The head is where a site declares its own identity. On a page carrying
// somebody else's writing, whoever wrote the body also wrote the tags around
// it, and a description down there is not the site's claim about itself.
func TestADescriptionInTheBodyIsNotTheSitesOwnClaim(t *testing.T) {
	doc := `<html><head><title>Real</title></head><body>
<meta name="description" content="Injected by a commenter." />
Words enough to read.</body></html>`
	page, err := testFetcher().FetchPage(context.Background(), serveHTML(t, doc).URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.HeadText) != 0 {
		t.Fatalf("Page.HeadText = %q, want none — the body is not the head", page.HeadText)
	}
}

// A tag stuffed with a page of prose would spend a prompt budget that belongs
// to the site's own words.
func TestAnOverlongDeclarationIsCutToTheBudget(t *testing.T) {
	doc := `<html><head><meta name="description" content="` +
		strings.Repeat("ü", headTextRunes+500) + `" /></head><body>Words enough.</body></html>`
	page, err := testFetcher().FetchPage(context.Background(), serveHTML(t, doc).URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.HeadText) != 1 {
		t.Fatalf("HeadText = %q, want one declaration", page.HeadText)
	}
	if got := len([]rune(page.HeadText[0])); got != headTextRunes {
		t.Fatalf("declaration = %d runes, want it cut to %d", got, headTextRunes)
	}
}

// Inline scripts are on pages that carry prose too; only a loaded bundle says
// the page expects a browser to build it.
func TestOnlyLoadedScriptsAreCounted(t *testing.T) {
	doc := `<html><head><script>window.x=1</script></head><body>Words enough to read.</body></html>`
	page, err := testFetcher().FetchPage(context.Background(), serveHTML(t, doc).URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if page.ExternalScripts != 0 {
		t.Fatalf("Page.ExternalScripts = %d, want 0 — an inline snippet loads nothing", page.ExternalScripts)
	}
}
