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

// retiredRecordSourceSpellings are the words that used to mean
// RecordSourceManual. `ui` named the screen and `mcp` named the transport;
// neither named an origin, which is the one thing this column says.
//
// `mcp` is still listed although nothing writes it and no row holds it: the
// sweep that retired it left nothing that fails when it returns, and `ui`
// has returned on exactly that footing before this gate existed to hold it.
var retiredRecordSourceSpellings = []string{"mcp", "ui"}

// RetiredRecordSourceSpellings lists the retired words, sorted. One owner, so
// the census over Go, the census over TypeScript and the migration predicate
// read the same set rather than three copies that drift.
func RetiredRecordSourceSpellings() []string {
	return slices.Clone(retiredRecordSourceSpellings)
}
