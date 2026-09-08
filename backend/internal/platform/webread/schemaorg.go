// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// What a page declares about itself in JSON-LD.
//
// A client-rendered site sends a loader and a <head>; the words a reader wants
// are assembled by a browser this fetcher does not run. The head's own prose
// (headtext.go) recovers some of it, and this recovers the rest: a marketing
// site built by any current framework ships a schema.org block naming the
// company behind it, served as text by the server, needing no browser at
// all.
//
// Its own file rather than headtext.go's, because the shape is different in the
// way that matters: a <meta> is one attribute to read, and this is a JSON
// document of arbitrary depth that arrived from a stranger. Everything below is
// bounded for that reason.

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// The bounds this reader answers within. All three exist because the input is
// untrusted: a page chooses its own JSON, and an unbounded parse of it would
// spend the crawl's memory and the prompt's budget on whatever a stranger sent.
const (
	// ldBlockBytes bounds ONE script block before it is parsed at all. Generous
	// against real product catalogs, far below what a page could send.
	ldBlockBytes = 256 * 1024
	// ldMaxDepth bounds how deep the walk descends. schema.org nests a few
	// levels; a document nesting hundreds is not describing a company.
	ldMaxDepth = 12
	// ldMaxClaims bounds what reaches a caller. The company's own name and
	// description are the first thing a page declares; the twentieth product in
	// a catalog is not what a reader asked.
	ldMaxClaims = 4
)

// ldClaimKeys are the fields worth reading off a node, in the order a reader
// wants them: what the thing is called, then what it says it does.
var ldClaimKeys = []string{"name", "legalName", "description"}

// linkedDataClaims reads the names and descriptions a page declares in JSON-LD.
//
// THE TYPE IS DELIBERATELY NOT FILTERED, and that is the decision worth reading.
// schema.org's Company vocabulary is OPEN — Corporation, NGO, and every
// LocalBusiness subtype down to Dentist are companies — so a list of the
// types to accept is a census that goes quietly short: a site declaring itself
// a `SportsActivityLocation` would be read as declaring nothing, and the read
// would report an empty page about a company that named itself in the markup.
//
// What bounds this instead is the CLAIM COUNT, which cannot fail that way. The
// consumer is a model reading prose, for which a page's own declared names are
// signal whatever node they hang off — and four of them is a sentence, not a
// catalog.
func linkedDataClaims(rawHTML string) []string {
	var claims []string
	seen := map[string]bool{}
	for _, block := range linkedDataBlocks(rawHTML) {
		var doc any
		if err := json.Unmarshal([]byte(block), &doc); err != nil {
			// A page's own malformed JSON is not this crawl's problem to report:
			// the block is one source of prose among several, and a site that
			// ships broken markup still has a <head> and a body to read.
			continue
		}
		claims = collectLDClaims(doc, seen, claims)
		if len(claims) >= ldMaxClaims {
			return claims
		}
	}
	return claims
}

// linkedDataBlocks returns the raw text of each application/ld+json script.
func linkedDataBlocks(rawHTML string) []string {
	var blocks []string
	tokenizer := html.NewTokenizer(strings.NewReader(rawHTML))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return blocks
		case html.StartTagToken:
			name, hasAttr := tokenizer.TagName()
			if string(name) != tagScript || !hasAttr {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(tagAttrs(tokenizer)["type"]), "application/ld+json") {
				continue
			}
			// The tokenizer hands script contents back as one text token, so the
			// block is whatever follows this tag.
			if tokenizer.Next() != html.TextToken {
				continue
			}
			block := string(tokenizer.Text())
			if len(block) > ldBlockBytes {
				continue
			}
			blocks = append(blocks, block)
		}
	}
}

// collectLDClaims reads the claims off every node of one decoded document,
// depth-first in document order. seen drops a repeated value: a site declaring
// its name on every node would otherwise spend the whole budget saying it again
// per node.
//
//craft:ignore naked-any the parameter IS a decoded JSON document of a shape nobody declares — see walkLinkedData
func collectLDClaims(doc any, seen map[string]bool, into []string) []string {
	walkLinkedData(doc, 0, func(node map[string]any) bool {
		for _, key := range ldClaimKeys {
			claim, ok := node[key].(string)
			if !ok {
				continue
			}
			claim = strings.Join(strings.Fields(claim), " ")
			if claim == "" || seen[claim] {
				continue
			}
			seen[claim] = true
			into = append(into, truncateRunes(claim, headTextRunes))
			if len(into) >= ldMaxClaims {
				return true
			}
		}
		return false
	})
	return into
}

// ldLogoKey is the schema.org field that names a company's logo — the
// one place a page says "this picture is our mark" in so many words, where a
// <link rel="icon"> only says "this is us at icon size".
const ldLogoKey = "logo"

// ldImageURLKeys are the fields an ImageObject carries its address in. A logo
// is declared either as a bare URL string or as such an object; both spellings
// are common and a reader that took only one would miss half the sites that
// declared one.
var ldImageURLKeys = []string{"url", "contentUrl"}

// linkedDataLogos reads the logos a page declares in JSON-LD, absolute and in
// document order, at most max of them. Like the claims, the type is not
// filtered: a LocalBusiness subtype's logo is its logo.
func linkedDataLogos(rawHTML string, base *url.URL, limit int) []string {
	var logos []string
	seen := map[string]bool{}
	for _, block := range linkedDataBlocks(rawHTML) {
		var doc any
		if err := json.Unmarshal([]byte(block), &doc); err != nil {
			continue
		}
		walkLinkedData(doc, 0, func(node map[string]any) bool {
			for _, ref := range ldImageRefs(node[ldLogoKey]) {
				resolved, ok := resolveAsset(base, ref)
				if !ok || seen[resolved] {
					continue
				}
				seen[resolved] = true
				logos = append(logos, resolved)
				if len(logos) >= limit {
					return true
				}
			}
			return false
		})
		if len(logos) >= limit {
			return logos
		}
	}
	return logos
}

// ldImageRefs reads the address(es) one logo value names: a URL string, an
// ImageObject, or a list of either. Nothing deeper — an ImageObject's own
// nested nodes describe the picture, they do not name a second one.
//
//craft:ignore naked-any the value IS a decoded JSON field of a shape nobody declares, the same reason collectLDClaims gives
func ldImageRefs(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case map[string]any:
		for _, key := range ldImageURLKeys {
			if ref, ok := typed[key].(string); ok {
				return []string{ref}
			}
		}
	case []any:
		var refs []string
		for _, item := range typed {
			refs = append(refs, ldImageRefs(item)...)
		}
		return refs
	}
	return nil
}

// walkLinkedData visits every object node of one decoded document, depth-first
// and bounded, and stops once visit reports it has what it came for.
//
// Both readers of a JSON-LD document — the claims and the logos — go through
// this walk, which keeps the depth bound and the visiting order together
// rather than in two copies. The order is stable on purpose: map iteration is randomized
// in Go, and a page whose declarations read differently between two crawls
// would make every downstream dedupe and evidence match depend on which order
// this walk happened to take.
//
// The node is `any` because it is a decoded JSON document of a shape nobody
// declares — schema.org is an open vocabulary and the page chooses which of it
// to send, so a concrete type here would be this package inventing a contract
// the input never agreed to. The walk answers only "is this a map, a list, or a
// string", which is the whole of what json.Unmarshal into `any` can be asked.
//
//craft:ignore naked-any the parameter IS a decoded JSON document of a shape nobody declares
func walkLinkedData(node any, depth int, visit func(map[string]any) bool) bool {
	if depth > ldMaxDepth {
		return false
	}
	switch value := node.(type) {
	case map[string]any:
		if visit(value) {
			return true
		}
		for _, key := range sortedKeys(value) {
			if walkLinkedData(value[key], depth+1, visit) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if walkLinkedData(item, depth+1, visit) {
				return true
			}
		}
	}
	return false
}

// sortedKeys is the walk's fixed order over a JSON object.
func sortedKeys(node map[string]any) []string {
	keys := make([]string, 0, len(node))
	for key := range node {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
