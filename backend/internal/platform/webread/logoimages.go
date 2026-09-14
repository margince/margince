// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// The pictures a page calls its logo.
//
// A site declares its ICONS in the <head> and its LOCKUP nowhere: the wordmark
// a company prints on its letterhead is an <img> in the page's header with no
// rel to name it. Two things do name it. A schema.org block names it in so many
// words (`logo`, schemaorg.go), and the page's own markup labels the image —
// in its alt text, its class, its id or the file's name — because that is how
// the contacts who built the site found it too.
//
// This harvest deliberately reads the BODY, which the <head>-only asset harvest
// (extractHeadAssets) refuses to, and the two are not in conflict: the body is
// read for the one asset that lives nowhere else, and only with the page's own
// label on it. A caller that must not let page content choose a company's face
// stays with the head's declarations; the one that reads this list shows the
// result to a contact before any record wears it.

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// maxDeclaredLogos bounds how many logos one page can put forward, across
// both spellings. The header's mark is the first labelled image on nearly every
// site; the fourth is a partner strip or a footer badge, and each one past the
// bound is a fetch the resolve would spend on it.
const maxDeclaredLogos = 3

// logoLabel is the word a page uses when it labels its own mark. Matched as a
// substring, case-insensitively, because the spellings are "site-logo",
// "Logo_Header", "acme-logo.svg" and "Acme Logo" — never one token.
const logoLabel = "logo"

// logoLabelAttrs are the attributes a page labels an image in, in the order
// they are worth reading: what the image is called for a reader, what the
// stylesheet calls it, what the script calls it, and what the file is called.
var logoLabelAttrs = []string{"alt", "class", "id", "src"}

// declaredLogos harvests the images a page names as its logo, in the order the
// page offers them: what its JSON-LD declares first, because that is the page
// saying "logo" in a vocabulary made for the purpose, then the <img> elements
// it labels as one. Absolute, deduplicated, at most maxDeclaredLogos.
func declaredLogos(rawHTML string, base *url.URL) []string {
	logos := linkedDataLogos(rawHTML, base, maxDeclaredLogos)
	return labelledLogoImages(rawHTML, base, logos)
}

// labelledLogoImages appends the <img> elements the page labels as a logo to
// into, up to the bound, skipping any already there.
func labelledLogoImages(rawHTML string, base *url.URL, into []string) []string {
	seen := make(map[string]bool, len(into))
	for _, u := range into {
		seen[u] = true
	}
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	for len(into) < maxDeclaredLogos {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			// io.EOF or a malformed tail: the parseable prefix has been
			// harvested, which is all a best-effort discovery aid owes.
			return into
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := tokenizer.TagName()
		if string(name) != "img" || !hasAttr {
			continue
		}
		src, ok := logoImageSource(tagAttrs(tokenizer), base)
		if !ok || seen[src] {
			continue
		}
		seen[src] = true
		into = append(into, src)
	}
	return into
}

// logoImageSource reads one <img>'s attributes as a logo declaration: the
// absolute address of an image the page labels as its logo, or nothing.
//
// The label is read off the attributes a page labels an image in, and the
// address must resolve to something fetchable: an inline data: URI is not an
// asset to fetch, and a lazy-loading placeholder that names no file is not the
// picture.
func logoImageSource(attrs map[string]string, base *url.URL) (string, bool) {
	if !labelledAsLogo(attrs) {
		return "", false
	}
	return resolveAsset(base, attrs["src"])
}

// labelledAsLogo reports whether any of the labelling attributes says logo.
func labelledAsLogo(attrs map[string]string) bool {
	for _, attr := range logoLabelAttrs {
		if strings.Contains(strings.ToLower(attrs[attr]), logoLabel) {
			return true
		}
	}
	return false
}
