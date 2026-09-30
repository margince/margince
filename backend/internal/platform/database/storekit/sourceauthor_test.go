// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The author columns are numbered after the statement's own binds, so a create
// with nineteen fixed args binds the author at $20 and $21 and a custom field
// chained after them at $22.
func TestTheAuthorPairIsNumberedAfterTheFixedBinds(t *testing.T) {
	seat := ids.NewV7()
	name := "Mutaz Suleiman"
	base := make([]any, 19)

	cols, holders, args := AuthorInsertFragments(SourceAuthorInput{AuthorID: &seat, AuthorName: &name}, base)

	if cols != ", source_author_id, source_author_name" {
		t.Errorf("cols = %q", cols)
	}
	if holders != ", $20, $21" {
		t.Errorf("placeholders = %q, want \", $20, $21\"", holders)
	}
	if len(args) != 21 || args[19] != &seat || args[20] != &name {
		t.Errorf("the pair is not bound at the end of the args: %d args", len(args))
	}
	if len(base) != 19 {
		t.Errorf("the caller's base slice was written through: len %d", len(base))
	}
}

// No author still binds both columns, as NULL, so every create spells one
// statement rather than two.
func TestNoAuthorBindsTwoNulls(t *testing.T) {
	_, holders, args := AuthorInsertFragments(SourceAuthorInput{}, []any{"x"})

	if holders != ", $2, $3" {
		t.Errorf("placeholders = %q", holders)
	}
	id, idOK := args[1].(*ids.UUID)
	name, nameOK := args[2].(*string)
	if !idOK || !nameOK || id != nil || name != nil {
		t.Errorf("an empty author must bind two typed NULLs, got %#v", args[1:])
	}
}
