// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reportdoc validates a report document against the closed block
// grammar.
//
// The rule the whole package exists for: a number a reader sees is
// dereferenced from a saved run's cell, never carried in the document. A
// composer — a person building a report, or a model drafting one — supplies
// the STRUCTURE and the WORDS. It never supplies a figure.
//
// That is why a numeric literal is refused even when a valid handle sits
// beside it. A block carrying both is the dangerous case rather than the
// harmless one: the two can disagree, the literal is what renders, and nobody
// downstream can tell that the number on the page is not the number the
// database computed. Refusing only the literal-without-handle case would admit
// exactly the failure the design exists to prevent.
//
// Validation is structural. It says whether a document is well formed and
// whether every number it shows is dereferenceable; it does not resolve the
// handles, because resolving is a read and a read belongs to whoever is
// reading, under their own grants.
package reportdoc
