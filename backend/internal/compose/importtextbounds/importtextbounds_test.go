// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importtextbounds

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
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
	if _, _, _, refused := Refusal(migration.ObjectContact, map[string]string{"title": atLimit}); refused {
		t.Fatal("a title at the limit was refused")
	}
	target, _, why, refused := Refusal(migration.ObjectContact, map[string]string{"title": atLimit + "界"})
	if !refused || target != "title" || why.Code != migration.ReasonValueTooLong {
		t.Fatalf("a title past the limit = %v %q %+v, want value_too_long on title", refused, target, why)
	}
}

// A name part at the typed limit lands; one past it is refused on its own cell.
func TestAContactNamePartPastTheLimitIsRefusedOnItsCell(t *testing.T) {
	atLimit := strings.Repeat("ü", contacts.ContactNameLimit)
	for _, target := range []string{"full_name", "first_name", "last_name"} {
		fields := map[string]string{"full_name": "Anna Weber", target: atLimit}
		if _, _, _, refused := Refusal(migration.ObjectContact, fields); refused {
			t.Fatalf("%s at the limit was refused", target)
		}
		fields[target] = atLimit + "x"
		got, _, why, refused := Refusal(migration.ObjectContact, fields)
		if !refused || got != target || why.Code != migration.ReasonValueTooLong {
			t.Fatalf("%s past the limit = %v %q %+v, want value_too_long on it", target, refused, got, why)
		}
	}
}

// Parts that fit alone but join past the limit are refused on the last-name
// column, the one a file maps, with the joined name as the value.
func TestNamePartsJoiningPastTheLimitAreRefusedOnTheLastName(t *testing.T) {
	limit := contacts.ContactNameLimit
	fits := map[string]string{"first_name": strings.Repeat("a", limit/2), "last_name": strings.Repeat("b", limit-limit/2-1)}
	if _, _, _, refused := Refusal(migration.ObjectContact, fits); refused {
		t.Fatalf("parts joining to %d characters were refused", limit)
	}
	first, last := strings.Repeat("a", limit*2/3), strings.Repeat("b", limit*2/3)
	target, value, why, refused := Refusal(migration.ObjectContact, map[string]string{"first_name": first, "last_name": last})
	if !refused || target != "last_name" || value != first+" "+last || why.Code != migration.ReasonValueTooLong {
		t.Fatalf("joined parts = %v %q %q %+v, want value_too_long on last_name with the joined name", refused, target, value, why)
	}
}
