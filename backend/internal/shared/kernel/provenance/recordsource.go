// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import "slices"

// RecordSourceManual is the one spelling a record carries when someone using
// this product wrote it — through a form or through an assistant, which are the
// same human with the same authority. Which door they came through is the
// passport's answer, and who walked through it is captured_by's.
//
// Held by: TestNoGoSourceLiteralSpellsARetiredWord (backend/gates/recordsourcespelling_test.go)
const RecordSourceManual = "manual"

// retiredRecordSourceSpellings are refused as a record's source: `ui` names a
// screen and `mcp` a transport, and neither names an origin, which is the one
// thing this column says. A word stays listed even when nothing writes it —
// an unlisted word is one nothing fails on, which is how a retired one returns.
var retiredRecordSourceSpellings = []string{"mcp", "ui"}

// RetiredRecordSourceSpellings lists the retired words, sorted. One owner, so
// the census over Go, the census over TypeScript and the migration predicate
// read the same set rather than three copies that drift.
func RetiredRecordSourceSpellings() []string {
	return slices.Clone(retiredRecordSourceSpellings)
}
