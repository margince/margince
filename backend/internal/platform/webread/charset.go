// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding/charmap"
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
	if !decodableText(parseMediaType(got.contentType)) || utf8.Valid(got.body) {
		return string(got.body)
	}
	if kept, cut := utf8CutByCap(got); cut {
		return string(kept)
	}
	sniff := got.body[:min(len(got.body), charsetSniffBytes)]
	encoding, name, _ := charset.DetermineEncoding(sniff, got.contentType)
	if name == "utf-8" || name == "windows-1252" {
		return utf8WithLegacyBytes(got.body)
	}
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

// utf8CutByCap returns a capped body without the multi-byte rune the cap cut
// short, and reports whether the body ends in such a rune.
//
// The rest must be valid UTF-8 with at least one non-ASCII rune. An ASCII body
// ending in one high byte is more likely Latin-1 than a cut rune.
func utf8CutByCap(got fetched) ([]byte, bool) {
	if !got.capped {
		return nil, false
	}
	body := got.body
	for i := len(body) - 1; i >= 0 && i >= len(body)-utf8.UTFMax; i-- {
		if !utf8.RuneStart(body[i]) {
			continue
		}
		if utf8.FullRune(body[i:]) {
			return nil, false
		}
		rest := body[:i]
		return rest, utf8.Valid(rest) && !isASCII(rest)
	}
	return nil, false
}

func isASCII(body []byte) bool {
	for _, b := range body {
		if b >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// utf8WithLegacyBytes reads body as UTF-8, and every byte outside a valid
// UTF-8 sequence as Windows-1252.
//
// A page built from a UTF-8 template with a Latin-1 snippet keeps both halves.
// A pure Latin-1 body comes out as a plain Windows-1252 decode would give it.
func utf8WithLegacyBytes(body []byte) string {
	var out strings.Builder
	out.Grow(len(body) + len(body)/8)
	for len(body) > 0 {
		r, size := utf8.DecodeRune(body)
		if r == utf8.RuneError && size == 1 {
			r = charmap.Windows1252.DecodeByte(body[0])
		}
		out.WriteRune(r)
		body = body[size:]
	}
	return out.String()
}
