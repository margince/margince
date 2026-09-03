// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// What a page SAYS about itself, and what it needs a browser for.
//
// Its own file because it is one concept the rest of page.go does not share:
// every other harvest there reads an ASSET — an image, an icon, a forwarding
// address — while these two read the page's prose and the shape of its markup.
// Together they are what lets a caller tell a client-rendered application
// shell from an empty document, which is a question about the site rather than
// about any file it serves.

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// headTextRunes bounds one harvested head declaration. A description is a
// sentence or two by convention; anything longer is a page stuffing the tag,
// and it would be spent out of a prompt budget that belongs to the page's own
// prose.
const headTextRunes = 1000

// headTextNames are the <head> declarations that say what a page is ABOUT,
// in the order a reader would want them: the description first, because it is
// written for a stranger, then the Open Graph pair a site keeps for sharing.
var headTextNames = []string{"description", "og:title", "og:description"}

// headTextFrom reads one <meta>'s attributes as a claim about what the page is
// about. Open Graph names its fields in `property` and some CMSs emit them in
// `name`, so both are accepted — the same rule ogImageFrom follows.
//
// seen carries the declarations already harvested, so a page repeating a tag
// contributes it once: first declaration wins, as it does for og:image, and a
// site that emits og:description twice must not spend the budget twice. A
// declaration whose content duplicates one already taken is dropped too — a
// title and an og:title are commonly identical, and reading the same sentence
// twice tells a reader nothing the first did not.
func headTextFrom(attrs map[string]string, seen map[string]bool) (string, bool) {
	var field string
	for _, candidate := range headTextNames {
		if attrs["property"] == candidate || attrs["name"] == candidate {
			field = candidate
			break
		}
	}
	if field == "" || seen[field] {
		return "", false
	}
	seen[field] = true
	content := strings.Join(strings.Fields(attrs["content"]), " ")
	if content == "" {
		return "", false
	}
	content = truncateRunes(content, headTextRunes)
	if seen["v:"+content] {
		return "", false
	}
	seen["v:"+content] = true
	return content, true
}

// truncateRunes cuts a string to at most n runes, counting characters rather
// than bytes so a multi-byte sentence is not split mid-rune.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// countExternalScripts counts the <script src=…> tags a page loads.
//
// It reads the whole document, not just the head: a client-rendered site puts
// its bundle wherever the build tool chose, and the question here is what the
// page needs a BROWSER for, not what it declares about itself. Inline scripts
// are not counted — an analytics snippet is on every page, including the ones
// that also carry prose.
func countExternalScripts(rawHTML string) int {
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	count := 0
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			return count
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := tokenizer.TagName()
		if string(name) != "script" || !hasAttr {
			continue
		}
		if tagAttrs(tokenizer)["src"] != "" {
			count++
		}
	}
}
