// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

// equivalentContent names, per namespace and version, a pair of digests that
// build the SAME schema: the content a database may already hold, and the exact
// source content it is equivalent to. A database holding the applied half is
// admitted while the source still hashes to the current half.
//
// BOTH halves are named on purpose. Keyed on the applied digest alone, an entry
// would admit whatever the source said next — the first edit would be excused,
// and so would every later one nobody checked. Naming the current half makes an
// entry expire the moment the file changes again: the next edit is a new claim,
// and it stops migrations until somebody checks it and records the new pair.
//
// This is an enumerated list of specific past content, not a rule that forgives
// editing. Every entry asserts that two exact byte sequences build the same
// schema, and belongs here only once that has been checked by comparing the
// schemas they produce. Nothing is added here to make an edit convenient.
//
// An entry admits the version on the way DOWN as well as up, so the two byte
// sequences must also ROLL BACK compatibly — the down half that ships will run
// against a database whose up half was the other sequence. The entry below
// satisfies that the simplest way there is: only the up file was edited, so the
// down halves are byte-identical. An entry whose down half differs needs that
// checked separately, and equal up-schemas do not imply it.
var equivalentContent = map[string]map[string][]equivalence{
	"core": {
		// 1789122755 is the migration that renamed the company record type's
		// columns. It rewrote stored row values BEFORE the PL/pgSQL function
		// bodies naming the renamed columns, so on a database holding rows the
		// first UPDATE fired a trigger whose body still spelled the pre-rename
		// column and the migration aborted. On an empty schema no trigger fires,
		// so that content applied cleanly and recorded the applied digest below.
		// Moving the function bodies ahead of the updates changed their order and
		// nothing else: the two files build schemas that differ in no catalog
		// object, compared as pg_dump -s of two databases migrated from them.
		"1789122755": {{
			applied: "1c6205d459b647ad8c3ae1cbac41ec19ad12d48e8186b2be2abdf8d68a59ed79",
			source:  "c3f55e24d2d4bb724f53d125e27b4d0a2410033015fd08dfd9b0a92e52898785",
		}},
	},
}

// equivalence is one checked claim that two byte sequences build one schema.
type equivalence struct {
	applied string // what the database recorded
	source  string // what the source must still hash to for that to hold
}
