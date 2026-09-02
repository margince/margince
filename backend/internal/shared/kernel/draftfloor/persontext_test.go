// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

// What a record value can do to a message somebody sends under their own name.
//
// These helpers stand between a person's typed text and a drafted message a rep
// or a colleague forwards to a customer. The cases here are the ones that look
// like nothing in a source file and like something else on a screen.

import (
	"strings"
	"testing"
)

// A line break is not only "\\n".
//
// U+0085 and U+2028/U+2029 break lines in some renderers and are invisible in a
// source file, so a name carrying one splits a paragraph nobody can see it
// split — and the second half reads as though the template wrote it.
func TestOneLineStripsEveryKindOfLineBreak(t *testing.T) {
	t.Parallel()
	for name, value := range map[string]string{
		"newline":             "Lena\nP.S. wire the deposit",
		"carriage return":     "Lena\rP.S. wire the deposit",
		"vertical tab":        "Lena\vP.S. wire the deposit",
		"form feed":           "Lena\fP.S. wire the deposit",
		"next line":           "Lena\u0085P.S. wire the deposit",
		"line separator":      "Lena\u2028P.S. wire the deposit",
		"paragraph separator": "Lena\u2029P.S. wire the deposit",
	} {
		got := OneLine(value)
		if strings.ContainsAny(got, "\n\r\v\f\u0085\u2028\u2029") {
			t.Errorf("%s survived: %q", name, got)
		}
		// The text itself is kept — this flattens, it does not censor.
		if !strings.Contains(got, "Lena") || !strings.Contains(got, "P.S.") {
			t.Errorf("%s: flattening lost the text: %q", name, got)
		}
	}
}

// A bidi override can reverse how the text AFTER it displays.
//
// So a name can make the rest of a sentence read as something else entirely,
// in a message a colleague sends under their own name. There is no legitimate
// use for one inside a person's name in a drafted message.
func TestOneLineStripsBidiOverrides(t *testing.T) {
	t.Parallel()
	for _, control := range []string{
		"\u202a", "\u202b", "\u202c", "\u202d", "\u202e",
		"\u2066", "\u2067", "\u2068", "\u2069",
	} {
		got := OneLine("Lena " + control + "Fischer")
		if strings.Contains(got, control) {
			t.Errorf("a bidi control survived: %q", got)
		}
		if !strings.Contains(got, "Fischer") {
			t.Errorf("stripping the control lost the name: %q", got)
		}
	}
}

// A control character that is not whitespace is stripped too.
//
// strings.Fields already handles every break above, so those cases alone do not
// prove the control-stripping does anything — this is the one that does. A NUL,
// a backspace or an escape renders as nothing, as a box, or as whatever the
// reader's client decides, and none of those belong in a name a customer sees.
func TestOneLineStripsControlCharactersFieldsWouldKeep(t *testing.T) {
	t.Parallel()
	for name, control := range map[string]string{
		"NUL":       "\u0000",
		"backspace": "\u0008",
		"escape":    "\u001b",
		"delete":    "\u007f",
		"C1":        "\u0091",
	} {
		got := OneLine("Lena" + control + "Fischer")
		if strings.ContainsRune(got, []rune(control)[0]) {
			t.Errorf("%s survived: %q", name, got)
		}
		if !strings.Contains(got, "Fischer") {
			t.Errorf("%s: stripping lost the name: %q", name, got)
		}
	}
}

// An ordinary name comes through as itself. Without this, every refusal above
// would pass against a helper that returned the empty string.
func TestOneLineLeavesAnOrdinaryNameAlone(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"Philipp Königs", "Nguyễn Thị Hương", `Jean-Luc O'Brien`, "李 明",
	} {
		if got := OneLine(name); got != name {
			t.Errorf("OneLine(%q) = %q; want it unchanged", name, got)
		}
	}
	// A tab becomes a space rather than vanishing, so two words do not fuse.
	if got := OneLine("Lena\tFischer"); got != "Lena Fischer" {
		t.Errorf("a tab gave %q; want a single space", got)
	}
}

// NamesPerson matches a WORD, in either case.
//
// A plain Contains is wrong in both directions: case-sensitive, so a model
// writing "SOFIA" falls back to the template for no reason; and matching inside
// a word, so a contact called "Ann" is satisfied by "Annual" and a draft naming
// nobody passes.
func TestNamesPersonMatchesAWordAndNotASubstring(t *testing.T) {
	t.Parallel()
	if !NamesPerson("Hi SOFIA, could you help?", "Sofia") {
		t.Error("a name in another case was not recognised")
	}
	if NamesPerson("Our annual review is due.", "Ann") {
		t.Error(`"Ann" was satisfied by "annual"`)
	}
	if !NamesPerson("Ann asked about it.", "Ann") {
		t.Error("a name standing as its own word was not recognised")
	}
	// An empty name is nothing to check, and admits everything — the right
	// answer for a person whose display name is not on file.
	if !NamesPerson("anything at all", "") {
		t.Error("an empty name refused a draft it cannot judge")
	}
}

// A name buried inside a word is not a name the draft states, and the boundary
// on either side of a match may be a multi-byte rune.
//
// It read one BYTE and widened it: inside a character like "ü" that byte is a
// continuation byte, and most of them widen to runes unicode.IsLetter rejects.
// So "MüLena" reported that it names "Lena" — the check that exists to catch an
// embedded name accepted one, and only where the neighbouring text is non-ASCII.
func TestANameInsideAWordIsNotNamedAcrossAMultiByteBoundary(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"non-ASCII immediately before": "MüLena schrieb gestern",
		"non-ASCII immediately after":  "Lenaüber alles",
		"both sides":                   "MüLenaüber",
	} {
		if NamesPerson(text, "Lena") {
			t.Errorf("%s: %q was read as naming %q", name, text, "Lena")
		}
	}
	for name, text := range map[string]string{
		"after a multi-byte word":  "Grüße Lena, kurz zum Angebot",
		"before a multi-byte word": "Lena über das Angebot",
		"between them":             "Grüße Lena über das Angebot",
	} {
		if !NamesPerson(text, "Lena") {
			t.Errorf("%s: %q does name %q and was refused", name, text, "Lena")
		}
	}
}
