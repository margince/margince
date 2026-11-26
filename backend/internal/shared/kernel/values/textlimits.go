// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

// The character limits a text column can have, from a short label to a document.
// An identity is the name or id another system gives a record.
const (
	TextLabelCharacters    = 120
	TextIdentityCharacters = 128
	TextLineCharacters     = 500
	TextNoteCharacters     = 2000
	TextDocumentCharacters = 20000
)
