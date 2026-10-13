// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extraction is the Tier-0 vocabulary for what reading a document
// produces: one field either grounded in the document's own words, or honestly
// omitted with the reason it was.
//
// It carries a TYPE and no interface. It used to hold an Extractor seam with a
// no-op production default, from when a reading was a function call made inside
// the request that asked for it. A reading is now a durable record the surface
// polls (records-depth RD-DDL-4, migrations/core/0252), because a model call
// takes seconds and can fail — so there is no call left to abstract, and the one
// thing two packages still have to agree on is the shape of a field.
//
// It lives at Tier-0 because that agreement crosses a module boundary: the
// activities store persists these and compose's reading produces them, and a
// module never imports a sibling.
package extraction

// ExtractedField is one attempted grounded field, or one omitted field when
// Omitted is true. A non-omitted field always carries the evidence that grounds
// it (SourceQuote/PageOrSection/Confidence) — GATE-AI-1's evidence-or-omit
// invariant: never a guessed value.
// The tags SPELL THE KEYS ALREADY STORED, which are Go's default field names:
// `attachment_extraction.fields` is full of rows written that way, and
// compose/dealscoutsql.go reads them by key. Tagging in lower_snake would orphan
// every stored row.
//
// Explicit is the point rather than the spelling: untagged, a rename silently changed
// what those predicates look for, with no compiler error and no failing test.
//
//nolint:tagliatelle // the keys are already in attachment_extraction.fields, written before anything tagged this; house style here would orphan every stored row
type ExtractedField struct {
	Field         string `json:"Field"`
	Value         string `json:"Value"`
	SourceQuote   string `json:"SourceQuote"`
	PageOrSection string `json:"PageOrSection"`
	Confidence    string `json:"Confidence"`
	Omitted       bool   `json:"Omitted"`
	OmittedReason string `json:"OmittedReason"`
}
