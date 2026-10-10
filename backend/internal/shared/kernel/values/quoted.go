// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "strings"

// Quoted reports whether a quote is the text's own words.
//
// Whitespace is collapsed on both sides before comparing, and only
// whitespace: a document's text arrives with the line breaks and column
// padding its layout happened to have, and a reply that reads a value off two
// lines writes it as one sentence. Normalizing more than that — case,
// punctuation, accents — would start admitting quotes the text does not
// contain, which is the one thing this check exists to refuse.
//
// An EMPTY quote never matches, and that guard lives here rather than at a
// caller. strings.Contains is true for the empty string against anything, so
// without it a reply that quoted nothing would be admitted everywhere — and
// "nothing" is exactly what a model reaches for when it has no span to point
// at.
//
// One spelling for every grounding check, the extractors' and a person's alike:
// two copies of the rule drift until one of them admits a quote the other
// would have refused.
func Quoted(text, quote string) bool {
	quote = CollapseSpace(quote)
	if quote == "" {
		return false
	}
	return strings.Contains(CollapseSpace(text), quote)
}

// CollapseSpace folds every run of whitespace into one space, which is the
// normalisation Quoted compares under and what a caller locating a quote in
// its text compares under too.
func CollapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }
