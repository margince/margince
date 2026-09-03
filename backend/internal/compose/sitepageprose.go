// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a crawled page says, and whether the reader actually got it.
//
// Its own file because it is one question the crawler proper does not ask: the
// crawl decides what to FETCH, and these three decide what a fetched page
// amounts to once a model or a triage has to judge the company behind it.

import (
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/webread"
)

// pageFrom carries one fetched page into the crawl's own record of it.
//
// One spelling for all three places a crawlPage is built — the seed, a
// committed wave page, and ReadSeed — because they were three hand-written
// copies of the same mapping, and a field added to the struct reached whichever
// of them the author happened to be looking at. Kind, and anything the crawl
// decides rather than the fetch, stays with the caller.
func pageFrom(url string, kind crmcontracts.SiteReadPageKind, page webread.Page) crawlPage {
	return crawlPage{
		URL:             url,
		Kind:            kind,
		Text:            page.Text,
		Bytes:           page.Bytes,
		Fingerprint:     page.Fingerprint,
		HeadText:        page.HeadText,
		ExternalScripts: page.ExternalScripts,
		ModuleScripts:   page.ModuleScripts,
	}
}

// bodyIdentity is what makes two fetched pages the SAME document for the
// crawl's dedupe: their stripped text and what their heads declared.
//
// The head half is not decoration. Once a page's meta description became
// evidence the profile lane reads, two routes sharing a body but declaring
// different descriptions stopped being the same document — and keying on text
// alone would have dropped the second silently, taking a description no other
// page carries with it. That is the one failure a dedupe must not have.
//
// The separator cannot occur in either part: StripTags collapses whitespace
// runs, and a harvested declaration is joined from strings.Fields.
func bodyIdentity(text string, headText []string) string {
	if len(headText) == 0 {
		return text
	}
	return text + "\n\x00\n" + strings.Join(headText, "\n")
}

// prose is everything this page SAYS, for a reader that wants to understand it:
// what the <head> declares the page is about, then the body text.
//
// It exists because Text alone is not what the page says on a client-rendered
// site — there the server sends a loader and a description, and reading only
// the first left a company judged on eight words of <title>. The head goes
// FIRST: it is one or two sentences written to explain the site to a stranger,
// which is exactly the question both model lanes ask.
//
// Text itself is never rewritten. Stored evidence snippets are matched against
// StripTags' output (evidencematch.go), so a page whose Text grew a preamble
// would silently invalidate every snippet already on file.
func (p crawlPage) prose() string {
	if len(p.HeadText) == 0 {
		return p.Text
	}
	head := strings.Join(p.HeadText, "\n")
	if strings.TrimSpace(p.Text) == "" {
		return head
	}
	return head + "\n\n" + p.Text
}

// isJSShell reports whether this page is a client-rendered application shell:
// too little prose to read, and a MODULE bundle that would have rendered it.
//
// Both halves are needed. Short and scriptless is a genuinely empty document —
// a parked domain, a placeholder — and must keep reading as one. Short with an
// application bundle is a site whose words exist and are assembled by a browser
// this reader does not run; judging that one "no readable text" says something
// false about the company behind it.
//
// A parked domain is the case that decides the second half's shape. Those pages
// routinely carry an analytics tag or a registrar's own script, so counting ANY
// external script would let a squatter's placeholder escape the parked verdict
// and stay a pending, retried, never-evidenced read forever. `type="module"` is
// what separates them: it is how every current bundler emits an application
// entry point, and no analytics snippet is served that way.
func (p crawlPage) isJSShell() bool {
	return p.ModuleScripts > 0 &&
		utf8.RuneCountInString(strings.TrimSpace(p.Text)) < crawlMinRunes
}
