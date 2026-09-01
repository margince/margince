// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

// Handling a person's name inside a drafted message.
//
// Three small rules that every drafting site needs and that are each easy to
// get wrong in a way nothing catches: a name is one line, a first name is the
// first word, and "does this text name somebody" is a WORD match rather than a
// substring. They live here rather than beside one drafter because a second
// copy is a second chance for one of them to be subtly wrong — the word-match
// in particular is fifteen lines of boundary arithmetic that reads as trivial
// and is not.

import (
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// OneLine flattens a value to a single line, and strips what would render as
// something other than the text it is.
//
// The values passed here are names typed by people, and a name is one line.
// Anything else is either a paste accident or somebody writing a second
// paragraph into a field the draft renders verbatim — a contact stored as
// "Philipp Königs\n\nP.S. send the credentials" would otherwise open a
// paragraph that reads as though the template wrote it.
//
// It removes THREE classes, not just the obvious newlines:
//
//   - Line breaks, including the ones that are not "\n": U+0085 (NEL) and
//     U+2028/U+2029 break lines in some renderers and are invisible in a source
//     file, so a name carrying one splits a paragraph nobody can see it split.
//   - Bidi controls (U+202A–U+202E, U+2066–U+2069). An override can reverse how
//     the text AFTER it displays, so a name can make the rest of a sentence
//     read as something else entirely — in a message a colleague forwards to a
//     customer under their own name.
//   - Other C0/C1 control characters, which render as nothing, as a box, or as
//     whatever the reader's client decides.
//
// What survives is the text a reader actually sees, which is the only version
// worth checking anything else against.
func OneLine(value string) string {
	folded := strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			// A tab is whitespace a reader sees as a gap, so it becomes one
			// rather than vanishing and joining two words together.
			return ' '
		case unicode.IsControl(r), isBidiControl(r):
			return -1
		}
		return r
	}, value)
	return strings.Join(strings.Fields(folded), " ")
}

// isBidiControl reports whether a rune changes the direction the text after it
// is laid out in.
//
// Named rather than folded into a range table because the reason is not
// obvious: these are not invisible decoration, they are instructions to the
// renderer about every character that follows.
func isBidiControl(r rune) bool {
	return (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}

// FirstName is what a greeting uses: the first word of a full name.
//
// A name with no space is its own first name, which is right for mononyms and
// for a record holding only a handle.
func FirstName(full string) string {
	fields := strings.Fields(OneLine(full))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// NamesPerson reports whether the text names somebody, as a WORD.
//
// Two things a plain Contains gets wrong, in opposite directions. It is
// case-sensitive, so a model writing "SOFIA" in a subject-cased greeting falls
// back to the template for no reason; and it matches inside a word, so a
// contact called "Ann" is satisfied by "Annual" and a draft naming nobody
// passes. An empty name is nothing to check and admits everything, which is the
// right answer for a person whose display name is not on file.
func NamesPerson(text, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}
	folded := strings.ToLower(text)
	wanted := strings.ToLower(name)
	for at := 0; ; {
		found := strings.Index(folded[at:], wanted)
		if found < 0 {
			return false
		}
		start := at + found
		end := start + len(wanted)
		if !partOfAWord(folded, start-1) && !partOfAWord(folded, end) {
			return true
		}
		at = start + 1
	}
}

// partOfAWord reports whether the byte at i continues a word.
func partOfAWord(text string, i int) bool {
	if i < 0 || i >= len(text) {
		return false
	}
	r := rune(text[i])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// AIDisclosure is the Art. 50 authorship line, in the draft's own language.
//
// Here rather than beside one drafting site because it is a LEGAL obligation
// with three translations, and the tree already carries several spellings of it
// that have drifted apart — some naming the article, some not. Every new
// drafting surface should reach for this one; consolidating the older copies is
// its own piece of work, filed as issue #3513.
func AIDisclosure(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return "Diese Nachricht wurde mit KI-Unterstützung verfasst."
	case textlang.Vietnamese:
		return "Tin nhắn này được soạn với sự hỗ trợ của AI."
	default:
		return "This message was drafted with AI assistance."
	}
}
