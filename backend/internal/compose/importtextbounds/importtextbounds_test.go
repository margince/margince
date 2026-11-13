// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importtextbounds

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestEveryBoundedTargetIsReadFromItsColumn(t *testing.T) {
	for _, tc := range []struct {
		object, target string
		limit          int
	}{
		{migration.ObjectContact, "title", values.TextLabelCharacters},
		{migration.ObjectCompany, "address.line1", values.TextLineCharacters},
		{migration.ObjectDeal, "name", values.TextLabelCharacters},
		{migration.ObjectDeal, "lost_reason", values.TextNoteCharacters},
		{migration.ObjectActivity, "subject", values.TextDocumentCharacters},
		{migration.ObjectContact, "cf_notes", values.TextDocumentCharacters},
		{migration.ObjectCompany, "display_name", 0},
		{migration.ObjectContact, "tags", 0},
	} {
		if got := limitOf(tc.object, tc.target); got != tc.limit {
			t.Errorf("%s.%s limit = %d, want %d", tc.object, tc.target, got, tc.limit)
		}
	}
}

func TestATitlePastTheLabelLimitIsRefused(t *testing.T) {
	atLimit := strings.Repeat("界", values.TextLabelCharacters)
	if _, _, refused := Refusal(migration.ObjectContact, map[string]string{"title": atLimit}); refused {
		t.Fatal("a title at the limit was refused")
	}
	target, why, refused := Refusal(migration.ObjectContact, map[string]string{"title": atLimit + "界"})
	if !refused || target != "title" || why.Code != migration.ReasonValueTooLong {
		t.Fatalf("a title past the limit = %v %q %+v, want value_too_long on title", refused, target, why)
	}
}
