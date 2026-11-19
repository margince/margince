// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// pageNamingRunes bounds a harvested name. A title is a line; a page stuffing
// the tag is not naming itself.
const pageNamingRunes = 300

// pageNaming is what a page's <head> calls the page and the site it is on.
type pageNaming struct {
	ogTitle  string
	tagTitle string
	siteName string
}

// title prefers og:title, which a site writes for a link to it shared
// elsewhere, over <title>, which often carries the site's name and a slogan.
func (n pageNaming) title() string {
	if n.ogTitle != "" {
		return n.ogTitle
	}
	return n.tagTitle
}

// readTitle takes the text of the first <title>, the tokenizer sitting on its
// start tag. Text already unescapes the entities of a <title>.
func (n *pageNaming) readTitle(tokenizer *html.Tokenizer) {
	if n.tagTitle != "" || tokenizer.Next() != html.TextToken {
		return
	}
	n.tagTitle = oneLine(string(tokenizer.Text()))
}

// readMeta takes og:title and og:site_name from one <meta>, first declaration
// winning. Open Graph names them in `property`, some CMSs in `name`.
func (n *pageNaming) readMeta(attrs map[string]string) {
	switch {
	case attrs["property"] == metaOGTitle || attrs["name"] == metaOGTitle:
		if n.ogTitle == "" {
			n.ogTitle = oneLine(attrs["content"])
		}
	case attrs["property"] == "og:site_name" || attrs["name"] == "og:site_name":
		if n.siteName == "" {
			n.siteName = oneLine(attrs["content"])
		}
	}
}

func oneLine(text string) string {
	return textcut.Runes(strings.Join(strings.Fields(text), " "), pageNamingRunes)
}
