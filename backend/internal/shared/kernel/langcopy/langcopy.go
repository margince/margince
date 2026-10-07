// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package langcopy holds a sentence in every language the product speaks.
//
// It sits in the shared tier because the writers that need it are spread from
// kernel helpers to compose engines, and a private copy in each would drift in
// its fallback and in what its census checks.
//
// Whole sentences per language, not clauses joined at runtime. An assembly
// like "They wrote last, " + about + ", and it is unanswered" survives
// translation only by accident: German puts the verb where English puts the
// object, and a glued sentence cannot move it.
package langcopy

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// Phrase is one sentence in every language, kept together so a translator reads
// the three side by side and a reviewer can see at a glance that they say the
// same thing.
type Phrase struct{ En, De, Vi string }

// In answers this sentence in one language, and in English for anything else.
func (p Phrase) In(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return p.De
	case textlang.Vietnamese:
		return p.Vi
	default:
		return p.En
	}
}

// Spoken is a table resolved to one language, so the writers below read a
// sentence rather than a lookup.
type Spoken struct {
	lang textlang.Lang
}

// Say answers one phrase in the resolved language.
func (s Spoken) Say(p Phrase) string { return p.In(s.lang) }

// Lang names the resolved language, for a writer that formats something the
// table cannot hold — a month name, a number's grouping.
func (s Spoken) Lang() textlang.Lang { return s.lang }

// For resolves a stored language code, falling back to English for anything
// this build does not speak.
//
// The fallback is not a guess: an unknown code reaches here only from a stored
// setting this build no longer ships, and English is what BaseLanguageOf itself
// answers for an installation that predates the setting. Writing in English is
// worse than writing in the reader's language and better than writing nothing.
func For(code string) Spoken {
	if textlang.Known(code) {
		return Spoken{lang: textlang.Lang(code)}
	}
	return Spoken{lang: textlang.English}
}
