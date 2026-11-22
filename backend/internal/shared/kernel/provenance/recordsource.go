// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"slices"
	"strings"
)

// RecordSourceManual is the one spelling a record carries when someone using
// this product wrote it — through a form or through an assistant, which are the
// same human with the same authority. Which door they came through is the
// passport's answer, and who walked through it is captured_by's.
//
// Held by: TestNoGoSourceLiteralSpellsARetiredWord (backend/gates/recordsourcespelling_test.go)
const RecordSourceManual RecordSource = "manual"

// RecordSource names the channel constants as one set for the schema gates. It
// is an alias, so a channel is a plain string wherever a source is written.
type RecordSource = string

// The other channels a record arrives by. Which system and which id inside it
// are source_system and source_id's to say, never this column's.
//
// Held by: TestTheRecordSourceVocabularyIsTheCatalogsCheck (backend/gates/recordsourcevocabulary_test.go)
const (
	// RecordSourceImport is a record loaded from another system's export or feed.
	RecordSourceImport RecordSource = "import"
	// RecordSourceCapture is a record read out of a connected mailbox, calendar or chat.
	RecordSourceCapture RecordSource = "capture"
	// RecordSourceAPI is a record an integration wrote through the public API.
	RecordSourceAPI RecordSource = "api"
	// RecordSourceAutomation is a record the product's own rules or agents opened.
	RecordSourceAutomation RecordSource = "automation"
	// RecordSourceWebform is a record a visitor's website form submitted.
	RecordSourceWebform RecordSource = "webform"
)

var recordSources = []string{
	RecordSourceAPI, RecordSourceAutomation, RecordSourceCapture,
	RecordSourceImport, RecordSourceManual, RecordSourceWebform,
}

// RecordSources lists the channels a constrained record table accepts, sorted.
func RecordSources() []string { return slices.Clone(recordSources) }

// RecordSourceError refuses a source outside the list on a create wire. It is
// caller-fixable, so every surface answers it as a 422 naming the field.
type RecordSourceError struct{}

func (e *RecordSourceError) Error() string {
	return "source must be one of " + strings.Join(recordSources, ", ") +
		"; name the other system in source_system and its id in source_id"
}

// FieldFault states the refusal for the HTTP mapper and the tool surface alike.
func (e *RecordSourceError) FieldFault() (field, code, message string) {
	return "source", "invalid_source", e.Error()
}

// AdmitRecordSource refuses a source the record tables' CHECK would refuse.
func AdmitRecordSource(source string) error {
	if !slices.Contains(recordSources, source) {
		return &RecordSourceError{}
	}
	return nil
}

// retiredRecordSourceSpellings are refused as a record's source: `ui` names a
// screen and `mcp` a transport, and neither names an origin, which is the one
// thing this column says. A word stays listed even when nothing writes it —
// an unlisted word is one nothing fails on, which is how a retired one returns.
var retiredRecordSourceSpellings = []string{"mcp", "ui"}

// RetiredRecordSourceSpellings lists the retired words, sorted. One owner, so
// the census over Go and the census over TypeScript read the same set rather
// than two copies that drift.
func RetiredRecordSourceSpellings() []string {
	return slices.Clone(retiredRecordSourceSpellings)
}

// ListedRecordSourceSQL is the test the record tables' CHECK makes, as a SQL
// expression over column. column is a compile-time SQL expression.
func ListedRecordSourceSQL(column string) string {
	return column + " IN ('" + strings.Join(recordSources, "', '") + "')"
}

// LegacyRecordSourceChannelSQL names the channel of a source written before
// the list was fixed, through the database's own mapping. column is a
// compile-time SQL expression.
func LegacyRecordSourceChannelSQL(column string) string {
	return "record_source_channel(" + column + ")"
}
