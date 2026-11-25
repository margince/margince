// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// formalAddress is the Sie register mid-sentence. A capital "Sie" that opens a
// sentence may mean "they", so only the forms that cannot are matched there.
var formalAddress = regexp.MustCompile(`(?:[^.?!:„(]\s)Sie\b|\b(?:Ihnen|Ihr|Ihre|Ihrem|Ihren|Ihrer|Ihres)\b`)

// GermanStyle fails a German sentence of a member-facing table that breaks a
// mechanical rule of docs/reference/ui-copy-style-de.md: a long dash, a
// semicolon, or the formal address. Copy written to an outsider (a contact, a
// buyer, a booking guest) is in Sie and does not call this.
//
//craft:ignore naked-any the table is the caller's own copy struct or map, as for Census
func GermanStyle(t reporter, table any) {
	t.Helper()
	walk(t, table, germanStyleEntry)
}

func germanStyleEntry(t reporter, name string, p langcopy.Phrase) {
	t.Helper()
	GermanText(t, name, p.In(textlang.German))
}

// GermanText is GermanStyle for one German sentence of a table that is not
// made of phrases, such as a struct of strings per language.
func GermanText(t reporter, name, german string) {
	t.Helper()
	GermanPunctuation(t, name, german)
	if formalAddress.MatchString(german) {
		t.Errorf("de %s addresses a member as Sie; the product says du: %q", name, german)
	}
}

// GermanPunctuation holds the page's punctuation rules alone, for a table
// whose sentences go to readers in either register.
func GermanPunctuation(t reporter, name, german string) {
	t.Helper()
	if strings.ContainsAny(german, "–—") || strings.Contains(german, "--") {
		t.Errorf("de %s uses a long dash; rewrite with a comma, period, colon or parentheses: %q", name, german)
	}
	if strings.Contains(german, ";") {
		t.Errorf("de %s joins two facts with a semicolon; split the sentence: %q", name, german)
	}
}
