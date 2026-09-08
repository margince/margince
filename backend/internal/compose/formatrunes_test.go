// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The runes a crawled page can carry that a reader never sees.
//
// They reach this tier because StripTags decodes character references, so a
// page writing `&#x202E;` hands over a real RIGHT-TO-LEFT OVERRIDE. What it
// costs is specific: an evidence snippet is rendered verbatim in a blockquote
// beside a decision somebody is being asked to make, and an override there
// makes the visible order differ from the stored one — the quotation a reader
// approves is not the quotation on file.

import (
	"strings"
	"testing"
)

func TestAPassageKeepsWhatAReaderSeesAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"ordinary text is returned untouched", "Acme GmbH, Kiel", "Acme GmbH, Kiel"},
		{"a right-to-left override", "HRB \u202e123456", "HRB 123456"},
		{"the whole bidi family", "a\u202a\u202b\u202c\u202d\u202eb", "ab"},
		{"the isolates too", "a\u2066\u2067\u2068\u2069b", "ab"},
		{"a zero-width space breaking a word", "Analytical\u200bEngines", "AnalyticalEngines"},
		{"a zero-width joiner and non-joiner", "a\u200c\u200db", "ab"},
		{"a byte-order mark", "\ufeffAcme", "Acme"},
		{"a C0 control", "Acme\x01GmbH", "AcmeGmbH"},
		// Whitespace is a SEPARATOR every caller already handles, and folding
		// it in here would silently change how a passage is segmented.
		{"a newline survives", "one\ntwo", "one\ntwo"},
		{"a tab survives", "one\ttwo", "one\ttwo"},
		{"an ordinary space survives", "one two", "one two"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := withoutFormatRunes(tc.in); got != tc.want {
				t.Errorf("withoutFormatRunes(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// And the comparison side, which has to agree.
//
// The token comparison (`contentTokens`) already splits on anything that is not
// a letter or a digit, so a format rune was never a difference to it. The
// containment one is where it counts: `evidenceOnPage` falls back to a
// substring test over the normalized text, and there a zero-width space on one
// side and not the other is a quotation the gate refuses although the page
// prints it.
func TestAnInvisibleRuneDoesNotStopASnippetMatchingItsPage(t *testing.T) {
	const pageText = "Impressum. Acme GmbH, Amtsgericht Kiel HRB 123456."
	pageNorm := normalizeEvidence(pageText)

	// The model quotes the page and its copy carries a zero-width space and an
	// override the page does not have — or the page carries them and the quote
	// does not. Neither is a difference a reader could see.
	if !evidenceOnPage(pageText, pageNorm, "Amtsgericht Kiel HRB \u200b123456") {
		t.Error("a quotation carrying a zero-width space matched nothing on the page that prints it")
	}
	if !evidenceOnPage(pageText, pageNorm, "Amtsgericht Kiel \u202eHRB 123456") {
		t.Error("a quotation carrying a bidi override matched nothing on the page that prints it")
	}
	// And the gate still refuses a quotation the page does NOT print: stripping
	// what a reader cannot see must not widen what counts as evidence.
	if evidenceOnPage(pageText, pageNorm, "Amtsgericht Hamburg HRB 123456") {
		t.Error("a quotation naming a court the page does not print was accepted as evidence")
	}
}

// A snippet built from a page carrying one is stored without it, which is the
// half normalizeEvidence cannot reach: what is rendered is the passage, not its
// normalized form.
func TestASnippetIsStoredWithoutTheRunesAReaderCannotSee(t *testing.T) {
	idx := newSnippetIndex(excerptPages{{
		URL:  "https://acme.example/impressum",
		Text: "Impressum. Acme GmbH, Amtsgericht Kiel HRB \u202e123456\u202c.",
	}})
	if len(idx.refs) == 0 {
		t.Fatal("the page produced no passage at all")
	}
	for _, ref := range idx.refs {
		if strings.ContainsRune(ref.passage, '\u202e') || strings.ContainsRune(ref.passage, '\u202c') {
			t.Errorf("a stored passage carries a bidi control: %q — rendered verbatim in a "+
				"blockquote, it can show a reader an order the bytes do not have", ref.passage)
		}
	}
}
