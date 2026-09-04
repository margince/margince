// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What a report answers with, held apart from the record shapes beside it.
//
// Its own file because it is the one result whose ROWS are not a declared
// shape: every other tool answers records this package spells out, and this one
// answers whatever plan the caller sent. What can be promised is the envelope,
// and what has to be SAID rather than promised — which currency the money is
// in, and which rows it leaves out — is the reason this carries more prose than
// a struct usually earns.

import "encoding/json"

// RunReportResult is the envelope every report answers with.
//
// Only the ROWS are dynamic. A report's columns come from the plan the caller
// sent, so `rows` is declared as objects and nothing is said about their
// members — but the envelope around them is the same for every report, and
// declaring it is what lets a caller find the columns, read the row count, and
// follow the drill-through handle without calling once to find out.
//
// The engine owns this shape; the members here are the ones its contract makes
// required, so this is a guaranteed subset like the passthroughs and the
// conformance suite holds it to a real report.
type RunReportResult struct {
	Report  string   `json:"report"`
	Columns []string `json:"columns"`
	// Plan is the validated query plan that ran — the caller's own request, back
	// in the words the engine accepted, so a model can see what its arguments
	// were understood to mean.
	Plan json.RawMessage `json:"plan"`
	// Rows are aggregate rows whose members ARE the columns above. Their shape
	// is the plan's, which is why nothing is declared about them here.
	//
	// One thing about their CONTENT is worth stating, because a model cannot
	// infer it and the number looks complete either way: a money measure
	// converted to the base currency EXCLUDES any deal whose currency has no
	// applicable rate, while the row count still counts it. Guessing a rate
	// would invent money, so the total is short on purpose — and a report that
	// offers a converted measure offers `count` OF that measure beside it,
	// which is how many rows the money actually covers. An agent summing the
	// one without reading the other reports a partial total as a whole one.
	Rows []json.RawMessage `json:"rows"`
	// TotalRows and DerivationURL are absent on a report that carries neither;
	// the handle is what "explain this number" follows.
	TotalRows     *int    `json:"total_rows,omitempty"`
	DerivationURL *string `json:"derivation_url,omitempty"`
	GeneratedAt   *string `json:"generated_at,omitempty"`
	// ExcludedByPermission is present when a field mask withheld rows from
	// this run — the count of visible rows excluded from every aggregate, so
	// a smaller total reads as governed rather than as missing data.
	ExcludedByPermission *int `json:"excluded_by_permission,omitempty"`
	// BaseCurrency is what a converted money measure is denominated in.
	//
	// The payload has always carried it; this struct did not declare it, so a
	// model reading the schema saw a figure with no currency and had to guess
	// or ask. A converted total labelled with the wrong currency is worse than
	// an unlabelled one, and there was nothing in the declared shape to stop
	// that guess being made.
	BaseCurrency *string `json:"base_currency,omitempty"`
}
