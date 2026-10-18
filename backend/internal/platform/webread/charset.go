// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
)

// charsetSniffBytes is how far into a body a meta charset declaration is
// honored, the same window the HTML standard gives a browser's prescan.
const charsetSniffBytes = 1024

// text is the body as UTF-8, which every parser downstream assumes. Only a
// text type is decoded; JSON is UTF-8 by definition.
//
// A valid UTF-8 body is kept even under a declared Latin charset. Misconfigured
// servers send that header over UTF-8 pages, and decoding would garble them.
func (got fetched) text() string {
	if !decodableText(parseMediaType(got.contentType)) {
		return string(got.body)
	}
	body := got.body
	if got.capped {
		body = withoutCutRune(body)
	}
	if utf8.Valid(body) {
		return string(body)
	}
	sniff := got.body[:min(len(got.body), charsetSniffBytes)]
	encoding, _, _ := charset.DetermineEncoding(sniff, got.contentType)
	decoded, err := encoding.NewDecoder().Bytes(got.body)
	if err != nil {
		// A decoder that refuses its input leaves the body as it arrived. Text
		// with a few broken letters is worth more to a caller than none.
		return string(got.body)
	}
	return string(decoded)
}

// decodableText reports whether a media type is text that may arrive in a
// legacy charset.
func decodableText(mediaType string) bool {
	if mediaType == "text/json" {
		return false
	}
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/xhtml+xml"
}

// withoutCutRune drops a multi-byte rune the size cap cut short at the end of
// body. Without this a capped UTF-8 page would fail the validity check on its
// last few bytes and be decoded as Windows-1252 throughout.
func withoutCutRune(body []byte) []byte {
	for i := len(body) - 1; i >= 0 && i >= len(body)-utf8.UTFMax; i-- {
		if utf8.RuneStart(body[i]) {
			if utf8.FullRune(body[i:]) {
				return body
			}
			return body[:i]
		}
	}
	return body
}
