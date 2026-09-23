// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import "slices"

// RecordSourceManual is the one spelling a record carries when a person using
// this product wrote it — through a form or through an assistant, which are the
// same person with the same authority. Which door they came through is the
// passport's answer, and who walked through it is captured_by's.
const RecordSourceManual = "manual"

// retiredRecordSourceSpellings are the words that used to mean
// RecordSourceManual. `ui` named the screen and `mcp` named the transport;
// neither named an origin, which is the one thing this column says.
//
// `mcp` is still listed although nothing writes it and no row holds it: the
// sweep that retired it left nothing that fails when it returns, and `ui`
// returned eight times on exactly that footing.
var retiredRecordSourceSpellings = []string{"mcp", "ui"}

// RetiredRecordSourceSpellings lists the retired words, sorted. One owner, so
// the census over Go, the census over TypeScript and the migration predicate
// read the same set rather than three copies that drift.
func RetiredRecordSourceSpellings() []string {
	return slices.Clone(retiredRecordSourceSpellings)
}
