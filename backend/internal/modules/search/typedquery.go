// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"strings"
	"unicode"
)

// normalizeQuery is what Search hands the splitter.
//
// Leading whitespace is the reader's slip; TRAILING whitespace is a statement —
// it is what says a word is finished. Trimming both, which Search did, made
// "a finished word" unreachable: "ann " became "ann", read as a fragment, and
// searched as `ann:*`, so a completed search silently reached "annual".
//
// Spelled here rather than at the call site so a test can drive the same
// normalisation the product does; asserting against its own copy is how the
// trailing-space case passed while the product had already erased it.
func normalizeQuery(raw string) string {
	return strings.TrimLeft(raw, " \t\r\n")
}

// splitTypedQuery divides a query into the words the reader FINISHED and the
// fragment they are still typing.
//
// Only the fragment is offered as a prefix. "automation wor" reaches a record
// carrying the whole word "automation" and something starting with "wor";
// loosening the earlier word too would return records the reader had already
// ruled out by finishing it.
//
// THE WHOLE QUERY IS THE HEAD when there is nothing to complete, and there are
// three such cases. A query ending in a space is a reader who finished a word.
// A closing quote says the same more strongly: the quotes mean those words
// entire. And a query using the websearch OPERATORS is one the reader is
// composing rather than typing a name into — `websearch_to_tsquery` gives them
// `or`, `-term` and quoted phrases, and splitting the string underneath that
// grammar tears an operator off its operand: "automation or katrin" would ask
// for the whole words "automation or" AND anything starting with "katrin",
// which is neither what they typed nor what they meant.
//
// The cost of the operator rule is that a reader mid-word in an operator query
// waits for the space, which is the same wait every query had before.
func splitTypedQuery(query string) (head, tail string) {
	if strings.HasSuffix(query, `"`) || carriesOperators(query) {
		return query, ""
	}
	i := strings.LastIndexFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || r == '"'
	})
	if i < 0 {
		return "", query
	}
	return query[:i], query[i+1:]
}

// carriesOperators reports whether the query uses the websearch grammar rather
// than being a run of plain words. `websearch_to_tsquery` reads a bare `or`
// between words as a disjunction, a leading `-` as a negation, and a quote as
// the start of a phrase — each of which binds to the words around it and so
// must not be cut in half.
func carriesOperators(query string) bool {
	if strings.Contains(query, `"`) {
		return true
	}
	for _, field := range strings.Fields(query) {
		if strings.HasPrefix(field, "-") || strings.EqualFold(field, "or") {
			return true
		}
	}
	return false
}

// prefixArmSQL matches the fragment the reader is still typing as a PREFIX.
//
// `plainto_tsquery` is the escaper, NOT `quote_literal`. The latter quotes for
// SQL, not for tsquery: given a fragment carrying a backslash it emits an
// E-string, and the `E` is then parsed as a lexeme of its own — "ACME\Sales"
// searched for a phantom `e` alongside the words the reader typed.
// `plainto_tsquery` renders the same input as `'acme' & 'sales'`, sanitized
// into lexemes with every operator already neutralized, and it never raises on
// text off a request.
//
// `:*` is appended to the rendered TEXT, so it marks the final lexeme. A
// fragment is one word by construction and usually renders one lexeme; when a
// separator inside it yields more ("ACME\Sales" → two), marking the last is
// exactly right — those are words the reader finished typing and the last is
// the one still under the cursor.
//
// An EMPTY fragment renders an empty tsquery, and `”:*` would be a syntax
// error, so the caller omits the arm entirely rather than building one. That is
// also what makes a finished query match exactly what it matched before.
func prefixArmSQL(tailPos int) string {
	return fmt.Sprintf(
		`(plainto_tsquery('simple', f_unaccent($%d))::text || ':*')::tsquery`,
		tailPos)
}

// matchExpression is the tsquery one branch matches against: the words the
// reader FINISHED, whole, AND the fragment they are still typing, as a prefix.
//
// AND rather than OR is the correctness of it. OR-ed, the prefix alone
// satisfies the match, so "zebra wor" reaches a record carrying neither word
// and typing MORE widens the answer instead of narrowing it.
//
// The `simple` parse is unaccented, so "Muller" finds "Müller", and it is
// OR-ed with the apostrophe-collapsed parse so "o'reilly" reaches a row stored
// as "OReilly". Both halves rest on the same invariant: every searchable table's
// `search_tsv` carries the unaccented and apostrophe-collapsed tokens, so a
// query parsed either way meets an index that speaks it.
//
// The activity branch parses German and English as well as `simple`, so rows
// whose tsvector stemmed "Verträge" answer a search for "Vertrag". Both halves
// carry those parses: a prefix compares `simple` lexemes while the index stores
// stems, so a fragment offered as `study:*` reaches neither the stored `studi`
// nor the simple `studies` — and every search is a fragment as it is typed.
func matchExpression(entity string, headPos, tailPos int, hasFragment bool) string {
	wholeWords := fmt.Sprintf(
		`websearch_to_tsquery('simple', f_unaccent($%[1]d)) || websearch_to_tsquery('simple', f_fold_apostrophes($%[1]d))`,
		headPos)
	if entity == entityActivity {
		wholeWords += fmt.Sprintf(
			` || websearch_to_tsquery('german', f_unaccent($%[1]d)) || websearch_to_tsquery('english', f_unaccent($%[1]d))`,
			headPos)
	}
	if !hasFragment {
		return fmt.Sprintf(`(%s)`, wholeWords)
	}
	fragment := prefixArmSQL(tailPos)
	if entity == entityActivity {
		fragment = fmt.Sprintf(
			`(%s || websearch_to_tsquery('german', f_unaccent($%[2]d)) || websearch_to_tsquery('english', f_unaccent($%[2]d)))`,
			fragment, tailPos)
	}
	return fmt.Sprintf(`((%s) && %s)`, wholeWords, fragment)
}
