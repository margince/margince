// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "strings"

// Quoted reports whether a quote is the text's own words, comparing under
// collapsed whitespace and nothing else. Case, punctuation and accents stay
// significant: folding them would admit quotes the text does not contain.
//
// An empty quote never matches, because strings.Contains is true for it
// against any text. This is the one grounding rule for the extractors and for
// a manual write.
func Quoted(text, quote string) bool {
	quote = CollapseSpace(quote)
	if quote == "" {
		return false
	}
	return strings.Contains(CollapseSpace(text), quote)
}

// CollapseSpace folds each run of whitespace into one space, the form Quoted
// compares under.
func CollapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }
