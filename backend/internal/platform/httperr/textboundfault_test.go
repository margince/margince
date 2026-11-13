// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestDatabaseTextBoundsUseTheSameFieldRefusalAsTypedWriters(t *testing.T) {
	for _, limit := range []int{values.TextLabelCharacters, values.TextLineCharacters, values.TextNoteCharacters, values.TextDocumentCharacters} {
		cause := &pgconn.PgError{Code: "22001", ConstraintName: "text_character_bound", ColumnName: "name", Detail: fmt.Sprint(limit), Message: "internal database wording"}
		fault := firstFieldFault(t, fmt.Errorf("write: %w", cause))
		if fault["field"] != "name" || fault["code"] != "too_long" || fault["max"] != float64(limit) {
			t.Fatalf("limit %d: field refusal = %+v", limit, fault)
		}
	}
}

func TestUnrecognizedDatabaseMetadataDoesNotBecomeAFieldRefusal(t *testing.T) {
	for _, cause := range []*pgconn.PgError{
		{Code: "22001", ColumnName: "name", Detail: "120"},
		{Code: "22001", ConstraintName: "text_character_bound", ColumnName: "name", Detail: "private database detail"},
		{Code: "22001", ConstraintName: "text_character_bound", ColumnName: "name", Detail: "121"},
		{Code: "22001", ConstraintName: "text_character_bound", ColumnName: "table.column", Detail: "120"},
		{Code: "23514", ConstraintName: "text_character_bound", ColumnName: "name", Detail: "120"},
	} {
		if _, ok := textBoundFault(cause); ok {
			t.Fatalf("unexpected public field metadata: %+v", cause)
		}
	}
}

func TestANamedTextCheckBecomesTheSameFieldRefusal(t *testing.T) {
	cause := &pgconn.PgError{Code: "23514", ConstraintName: storekit.TextMaxConstraint("cf_notes", values.TextDocumentCharacters), Message: "internal database wording"}
	fault := firstFieldFault(t, fmt.Errorf("write: %w", cause))
	if fault["field"] != "cf_notes" || fault["code"] != "too_long" || fault["max"] != float64(values.TextDocumentCharacters) {
		t.Fatalf("field refusal = %+v", fault)
	}
	for _, name := range []string{"cf_notes_check", "cf_notes_text_max_121", "cf_notes_text_max_"} {
		if _, ok := textBoundFault(&pgconn.PgError{Code: "23514", ConstraintName: name}); ok {
			t.Errorf("%s became a field refusal", name)
		}
	}
}
