// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package promptlang is the one spelling of "write in this language" for the
// prompts whose output the whole team reads.
//
// Held by: TestOnlyPromptlangSpellsTheLanguageRule (backend/promptlanguage_test.go)
//
// A model asked nothing about language answers in whatever language its input
// happened to be in. That is right for a reply to a German thread and wrong for
// a claim filed on a record: the thread has one reader who wrote it, the record
// has everyone. So a Vietnamese conversation produced a Vietnamese claim that a
// German colleague then had to read, and nothing in the product had ever
// decided otherwise — roughly two dozen prompts simply said nothing about
// language at all.
//
// This package is deliberately PURE. It renders a block of prompt text and
// nothing else: no settings read, no transaction, no context. The installation's
// base language is resolved by identity.BaseLanguageOf, which owns the setting,
// and passed in. A reader here would open its own workspace transaction while
// most callers already hold one, and would put a second answer to "what is the
// base language" beside the accessor that declares it.
//
// It does NOT govern every prompt. Correspondence keeps the language of the
// correspondence (compose/draftrules, which carries its own LANGUAGE block for
// exactly that reason), a brief cached for one reader keeps that reader's
// language, and a prompt whose output is data rather than prose needs no
// language rule at all.
package promptlang

import (
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Heading opens the rule, and it is what the fitness gate in
// backend/promptlanguage_test.go recognises a governed prompt by.
//
// Exported so that gate can READ it rather than restate it. A gate carrying its
// own copy of the string it looks for is a second spelling of this constant,
// and it goes quietly permissive the day this one changes — the check would
// still run, still pass, and no longer be about anything.
//
// draftrules.Shared opens with the same heading, which is what lets one gate
// recognise either: two blocks saying the same thing under one heading is one
// convention, where two headings would be two.
const Heading = "LANGUAGE\n"

// Rule is the language block to compose into a system prompt.
//
// It names the language in English ("Write in German") rather than passing the
// code, because that is what a model follows reliably. The carve-outs matter as
// much as the instruction: a model told to write in German will otherwise
// translate a company name, a status value or a JSON key, and a translated enum
// is a parse failure rather than a style problem.
//
// A code the product does not ship falls back to English rather than refusing.
// Every caller resolves the language through identity.BaseLanguageOf, whose
// entry validates against textlang.Shipped, so this is unreachable by any
// ordinary path — but the fallback exists because of what the alternatives cost.
// Refusing would take a whole brief down over a settings row somebody edited by
// hand; interpolating the unknown code would instruct the model in a language
// nobody named, which is the failure this package exists to remove. English is
// what these prompts produced before this package existed.
func Rule(lang string) string {
	return Heading + `Write every human-readable sentence of your output in ` + LanguageName(lang) + `.
Write naturally in that language rather than translating English phrasing.
Leave everything that is not a sentence exactly as it is given: JSON keys, enum
and status values, ids, urls, email addresses, people's names, company names,
and any text you are quoting from a source. Translating one of those changes
what it refers to.`
}

// LanguageName is the language's name in English, for any surface that tells
// a model which language to write in — the prompt rule below, and the tool
// surface that answers the same question to an agent running elsewhere.
//
// A code the product does not ship answers English, which is the fallback Rule
// documents above and the one every caller wants: the surfaces that call this
// hand the answer straight to a model, and a blank there instructs it in no
// language at all.
func LanguageName(lang string) string {
	if name := englishName(lang); name != "" {
		return name
	}
	return englishName(string(textlang.English))
}

// englishName is the language's name in English. Empty for anything the product
// does not ship, which is what LanguageName falls back on.
func englishName(lang string) string {
	switch textlang.Lang(lang) {
	case textlang.English:
		return "English"
	case textlang.German:
		return "German"
	case textlang.Vietnamese:
		return "Vietnamese"
	case textlang.Unknown:
		return ""
	default:
		return ""
	}
}
