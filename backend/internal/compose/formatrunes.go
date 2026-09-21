// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Runes a page can carry that are not what a reader sees.
//
// A crawled page reaches this tier through StripTags, which decodes character
// references — so `&#x202E;` arrives as U+202E RIGHT-TO-LEFT OVERRIDE and
// `&#x200B;` as a zero-width space, both invisible and both able to change what
// a quotation LOOKS like without changing a byte a gate compares.
//
// The one that matters is the bidi family. An evidence snippet is rendered
// verbatim in a blockquote beside a decision somebody is being asked to make,
// and an override there makes the visible order differ from the stored order:
// the quotation a reader approves is not the quotation on file. Zero-width
// characters are the quieter half of the same problem — they break a word for
// the eye and not for a comparison, or the reverse.
//
// Dropped rather than escaped, on the ground `normalizeEvidence` already drops
// the soft hyphen: a rendering hint is not content, and there is nothing here a
// reader loses.

import "unicode"

// formatRune reports a rune that carries no content: Unicode's Cf class (the
// bidi controls, the zero-width joiners and spaces, the byte-order mark) and
// the C0/C1 controls that survive text extraction.
//
// Whitespace is NOT this: a tab and a newline are separators every caller
// already handles, and folding them in here would silently change how a passage
// is segmented.
func formatRune(r rune) bool {
	if unicode.IsSpace(r) {
		return false
	}
	return unicode.Is(unicode.Cf, r) || unicode.IsControl(r)
}

// withoutFormatRunes drops them, leaving the text a reader would have seen.
func withoutFormatRunes(s string) string {
	// The overwhelmingly common case is a page with none, and a page's text is
	// long: scanning once and returning the original beats rebuilding it.
	clean := true
	for _, r := range s {
		if formatRune(r) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if !formatRune(r) {
			out = append(out, r)
		}
	}
	return string(out)
}
